// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	errNoContainersFound     = errors.New("no containers found in pod")
	errContainerNotFound     = errors.New("container not found in pod")
	errUnableToGetKubeconfig = errors.New("unable to get kubeconfig")
)

func exec(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	log.Debugf("[%s] exec with payload %+v", driver.Service, m.Payload)

	ctx, cancel := driver.GetContext(m)
	defer cancel()

	payload, _ := m.Payload.(map[string]any)
	if payload == nil {
		payload = map[string]any{}
	}

	// Get required parameters
	namespace, ok := dipper.GetMapDataStr(payload, "namespace")
	if !ok {
		namespace = DefaultNamespace
	}

	podName, ok := dipper.GetMapDataStr(payload, "pod")
	if !ok {
		m.Reply <- dipper.Message{Labels: map[string]string{"status": "error", "reason": "missing pod"}}

		return
	}

	containerName, _ := dipper.GetMapDataStr(payload, "container")
	// If container not specified, will use the first container in the pod

	shell, _ := dipper.GetMapDataStr(payload, "shell")
	if shell == "" {
		shell = "sh"
	}

	// Command can be provided as `command` (array) or `script` (string)
	var cmdArgs []string
	if c, ok := payload["command"]; ok && c != nil {
		switch t := c.(type) {
		case []any:
			for _, v := range t {
				cmdArgs = append(cmdArgs, fmt.Sprintf("%v", v))
			}
		case []string:
			cmdArgs = append(cmdArgs, t...)
		case string:
			cmdArgs = []string{shell, "-c", t}
		default:
			cmdArgs = []string{shell, "-c", fmt.Sprintf("%v", t)}
		}
	} else if s, ok := dipper.GetMapDataStr(payload, "script"); ok && s != "" {
		cmdArgs = []string{shell, "-c", s}
	} else {
		m.Reply <- dipper.Message{Labels: map[string]string{"status": "error", "reason": "missing command or script"}}

		return
	}

	k8client := prepareKubeConfig(m)

	// Execute the command
	exitCode, output, err := executeCommand(ctx, m, k8client, namespace, podName, containerName, cmdArgs)
	if err != nil {
		log.Errorf("[%s] exec failed: %+v", driver.Service, err)
		m.Reply <- dipper.Message{
			Labels: map[string]string{"status": "error", "reason": fmt.Sprintf("exec failed: %v", err)},
		}

		return
	}

	status := "success"
	reason := ""
	if exitCode != 0 {
		status = "failure"
		reason = fmt.Sprintf("exit code %d", exitCode)
	}

	log.Debugf("[%s] exec output: %s", driver.Service, output)

	m.Reply <- dipper.Message{
		Payload: map[string]any{
			"output":    output,
			"exit_code": exitCode,
		},
		Labels: map[string]string{"status": status, "reason": reason},
	}
}

func executeCommand(
	ctx context.Context,
	m *dipper.Message,
	k8client *kubernetes.Clientset,
	namespace, podName, containerName string,
	cmdArgs []string,
) (int, string, error) {
	// Get the pod to verify it exists and get container info
	coreV1 := k8client.CoreV1()
	pod, err := coreV1.Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return 0, "", fmt.Errorf("failed to get pod %s in namespace %s: %w", podName, namespace, err)
	}

	// If container name not specified, use the first container
	if containerName == "" {
		if len(pod.Spec.Containers) == 0 {
			return 0, "", fmt.Errorf("%w: %s", errNoContainersFound, podName)
		}
		containerName = pod.Spec.Containers[0].Name
	}

	// Verify container exists in pod
	containerFound := false
	for _, c := range pod.Spec.Containers {
		if c.Name == containerName {
			containerFound = true

			break
		}
	}
	if !containerFound {
		return 0, "", fmt.Errorf("%w: %s in %s", errContainerNotFound, containerName, podName)
	}

	// Get the rest config
	var restConfig *rest.Config
	source, _ := dipper.GetMapData(m.Payload, "source")
	stype, _ := dipper.GetMapDataStr(source, "type")

	switch stype {
	case "gcloud-gke":
		restConfig = getGKEConfig(source.(map[string]interface{}))
	case "local":
		restConfig, _ = rest.InClusterConfig()
	case "static":
		restConfig = getStaticKubeConfig(source.(map[string]interface{}))
	case "user":
		restConfig = getUserKubeConfig(source.(map[string]interface{}))
	}

	if restConfig == nil {
		return 0, "", errUnableToGetKubeconfig
	}

	// Create exec request
	req := coreV1.RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command:   cmdArgs,
			Container: containerName,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	// Create executor
	executor, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return 0, "", fmt.Errorf("failed to create executor: %w", err)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	streamOpts := remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  nil,
		Tty:    false,
	}

	// Execute the command
	err = executor.StreamWithContext(ctx, streamOpts)

	// Combine stdout and stderr
	output := stdout.String()
	if stderr.Len() > 0 {
		output += stderr.String()
	}

	// Determine exit code
	// Note: When the command exits with non-zero, Kubernetes returns an error
	// The executor will return an error if the command exits non-zero
	// We assume exit code 1 for any error since Kubernetes doesn't easily expose the actual exit code
	exitCode := 0
	if err != nil {
		exitCode = 1
		// Log the error for debugging but don't fail the exec handler
		log.Debugf("[%s] exec returned error: %v", driver.Service, err)
	}

	return exitCode, output, nil
}

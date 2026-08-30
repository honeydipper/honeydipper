// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// getPodInfoForJob retrieves pod information for a given job without blocking.
// Returns pod names and pod IPs if pods are currently running. If pods haven't been created yet,
// returns empty slices. This is useful for immediately getting pod names after job creation.
func getPodInfoForJob(ctx context.Context, k8client *kubernetes.Clientset, namespace, jobName string,
	targetPhase string,
) ([]map[string]string, bool) {
	podClient := k8client.CoreV1().Pods(namespace)
	listOpts := metav1.ListOptions{
		LabelSelector: "job-name==" + jobName,
	}

	pods, err := podClient.List(ctx, listOpts)
	if err != nil {
		log.Debugf("[%s] unable to list pods for job %s: %v", driver.Service, jobName, err)

		return []map[string]string{}, false
	}

	reached := targetPhase != "running"

	info := make([]map[string]string, len(pods.Items))
	for i, pod := range pods.Items {
		info[i] = map[string]string{
			"name":  pod.Name,
			"ip":    pod.Status.PodIP,
			"phase": string(pod.Status.Phase),
		}
		if targetPhase == "running" && pod.Status.Phase == v1.PodRunning {
			reached = true
		} else if targetPhase == "done" && pod.Status.Phase != v1.PodSucceeded && pod.Status.Phase != v1.PodFailed {
			reached = false
		}
	}

	return info, reached
}

func checkJobReachedPhase(ctx context.Context, k8client *kubernetes.Clientset, job *batchv1.Job, targetPhase string, m *dipper.Message) bool {
	// check job is in terminal state or not.
	terminal := false
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed || c.Type == batchv1.JobFailureTarget) &&
			c.Status == v1.ConditionTrue {
			terminal = true

			break
		}
	}

	// Check if job and pods already reached the designed phase.
	podInfo, reached := getPodInfoForJob(ctx, k8client, job.Namespace, job.Name, targetPhase)
	if targetPhase == "done" && !terminal {
		// pods complete does not mean job is complete,
		// ignore pod status until job is in terminal state.
		reached = false
	}
	if targetPhase == "running" && terminal {
		// job will never reach running phase any more. Quit now.
		reached = true
	}
	if len(podInfo) > 0 && reached || (len(podInfo) == 0 && terminal) {
		m.Reply <- dipper.Message{
			Labels: map[string]string{
				"status": "success",
			},
			Payload: map[string]interface{}{
				"pod_info": podInfo,
			},
		}

		return true
	}

	return false
}

// waitForJobPods watches a job until pods are created and returns their names.
// This is useful for waiting until a job has actually been scheduled and pods are available
// before running commands like exec against those pods.
func waitForJobPods(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}
	targetPhase, _ := dipper.GetMapDataStr(m.Payload, "target_phase")
	targetPhase = strings.ToLower(targetPhase)

	jobName := dipper.MustGetMapDataStr(m.Payload, "job")
	k8client := prepareKubeConfig(m)
	jobclient := k8client.BatchV1().Jobs(nameSpace)

	watchOption := metav1.ListOptions{
		FieldSelector: "metadata.name==" + jobName,
	}
	watchOption.Kind = "job"

	ctx, cancel := driver.GetContext(m)
	defer cancel()

	for {
		func() {
			joblist, err := jobclient.List(ctx, watchOption)
			if err != nil {
				log.Panicf("[%s] failed to get job info [%s]: %+v", driver.Service, jobName, err)
			}

			if len(joblist.Items) == 0 {
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
					// Job might not be created yet, retry after a delay.
				}

				return
			}
			job := joblist.Items[0]

			if checkJobReachedPhase(ctx, k8client, &job, targetPhase, m) {
				cancel()

				return
			}

			watchOption.ResourceVersion = joblist.ResourceVersion
			jobstatus, err := jobclient.Watch(ctx, watchOption)
			if err != nil {
				log.Panicf("[%s] unable to watch the job %+v", driver.Service, err)
			}
			defer jobstatus.Stop()

			for evt := range jobstatus.ResultChan() {
				if evt.Object == nil {
					return
				}

				if evt.Type == watch.Error || evt.Type == watch.Deleted {
					e := evt.Object.(*metav1.Status)
					if e.Code != http.StatusGone {
						log.Panicf("[%s] deleted or error from watching job [%s]: %+v", driver.Service, jobName, evt.Object)
					}
					cancel()

					return
				}

				log.Debugf("[%s] receiving event when watching for job [%s]", driver.Service, jobName)

				if job, ok := evt.Object.(*batchv1.Job); ok {
					if checkJobReachedPhase(ctx, k8client, job, targetPhase, m) {
						cancel()

						return
					}
				}
			}
		}()

		if ctx.Err() != nil {
			return
		}
	}
}

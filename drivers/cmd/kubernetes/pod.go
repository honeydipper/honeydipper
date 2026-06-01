// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package main

import (
	"context"
	"net/http"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// getPodInfoForJob retrieves pod information for a given job without blocking.
// Returns pod names and pod IPs if pods are currently running. If pods haven't been created yet,
// returns empty slices. This is useful for immediately getting pod names after job creation.
func getPodInfoForJob(ctx context.Context, k8client *kubernetes.Clientset, namespace, jobName string) ([]string, []string) {
	podClient := k8client.CoreV1().Pods(namespace)
	listOpts := metav1.ListOptions{
		LabelSelector: "job-name==" + jobName,
	}

	pods, err := podClient.List(ctx, listOpts)
	if err != nil {
		log.Debugf("[%s] unable to list pods for job %s: %v", driver.Service, jobName, err)

		return []string{}, []string{}
	}

	var podNames, podIPs []string
	for _, pod := range pods.Items {
		podNames = append(podNames, pod.Name)
		if pod.Status.PodIP != "" {
			podIPs = append(podIPs, pod.Status.PodIP)
		}
	}

	return podNames, podIPs
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
			if err != nil || len(joblist.Items) == 0 {
				log.Panicf("[%s] job not found [%s]: %+v", driver.Service, jobName, err)
			}
			watchOption.ResourceVersion = joblist.ResourceVersion

			// Check if pods are already created
			podNames, podIPs := getPodInfoForJob(ctx, k8client, nameSpace, jobName)
			if len(podNames) > 0 {
				m.Reply <- dipper.Message{
					Labels: map[string]string{
						"status": "success",
					},
					Payload: map[string]interface{}{
						"pod_names": podNames,
						"pod_ips":   podIPs,
					},
				}

				return
			}

			jobstatus, err := jobclient.Watch(ctx, watchOption)
			if err != nil {
				log.Panicf("[%s] unable to watch the job %+v", driver.Service, err)
			}
			defer jobstatus.Stop()

			for evt := range jobstatus.ResultChan() {
				if evt.Object == nil {
					return
				}

				if evt.Type == watch.Error {
					e := evt.Object.(*metav1.Status)
					if e.Code != http.StatusGone {
						log.Panicf("[%s] error from watching channel for job [%s]: %+v", driver.Service, jobName, evt.Object)
					}

					return
				}

				log.Debugf("[%s] receiving event when watching for job [%s]", driver.Service, jobName)

				// Check if pods are created yet
				podNames, podIPs := getPodInfoForJob(ctx, k8client, nameSpace, jobName)
				if len(podNames) > 0 {
					m.Reply <- dipper.Message{
						Labels: map[string]string{
							"status": "success",
						},
						Payload: map[string]interface{}{
							"pod_names": podNames,
							"pod_ips":   podIPs,
						},
					}
					cancel()

					return
				}
			}
		}()

		if ctx.Err() != nil {
			return
		}
	}
}

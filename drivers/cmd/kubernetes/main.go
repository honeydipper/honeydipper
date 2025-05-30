// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package kubernetes enables Honeydipper to interact with Kubernete clusters.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"dario.cat/mergo"
	"github.com/ghodss/yaml"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/rest"
)

const (
	// DefaultNamespace is the name of the default space in kubernetes cluster.
	DefaultNamespace string = "default"

	// StatusSuccess is the status when the job finished successfully.
	StatusSuccess = "success"

	// StatusFailure is the status when the job finished with error or not finished within time limit.
	StatusFailure = "failure"

	// DefaultJobWaitTimeout is the default timeout in seconds for waiting a job to be complete.
	DefaultJobWaitTimeout time.Duration = 10

	// LabelHoneydipperUniqueIdentifier is the name of the label to uniquely identify the job.
	LabelHoneydipperUniqueIdentifier = "honeydipper-unique-identifier"
)

var (
	log *logging.Logger
	err error
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Println("    This driver supports services including receiver, workflow, operator etc")
		fmt.Println("  This program provides honeydipper with capability of interacting with kuberntes")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "kubernetes")
	log = driver.GetLogger()
	driver.Commands["recycleDeployment"] = recycleDeployment
	driver.Commands["createJob"] = createJob
	driver.Commands["waitForJob"] = waitForJob
	driver.Commands["getJobLog"] = getJobLog
	driver.Commands["deleteJob"] = deleteJob
	driver.Commands["createPVC"] = createPVC
	driver.Commands["deletePVC"] = deletePVC
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func deleteJob(m *dipper.Message) {
	k8client := prepareKubeConfig(m)

	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}
	jobName := dipper.MustGetMapDataStr(m.Payload, "job")

	client := k8client.BatchV1().Jobs(nameSpace)
	ctx, cancel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
	defer cancel()
	deletePropagation := metav1.DeletePropagationBackground
	err := client.Delete(ctx, jobName, metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	if err != nil {
		log.Panicf("[%s] unable to delete the job %s: %+v", driver.Service, jobName, err)
	}
	m.Reply <- dipper.Message{
		Labels: map[string]string{
			"status": "success",
		},
	}
}

func getJobLog(m *dipper.Message) {
	k8client := prepareKubeConfig(m)

	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}
	jobName := dipper.MustGetMapDataStr(m.Payload, "job")

	listCtx, cancelList := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
	defer cancelList()
	jobclient := k8client.BatchV1().Jobs(nameSpace)
	watchOption := metav1.ListOptions{
		FieldSelector: "metadata.name==" + jobName,
	}
	joblist, err := jobclient.List(listCtx, watchOption)
	if err != nil || len(joblist.Items) == 0 {
		log.Panicf("[%s] job not found [%s]: %+v", driver.Service, jobName, err)
	}
	job := &joblist.Items[0]

	jobStatus, _, _ := getJobStatus(job)

	search := metav1.ListOptions{
		LabelSelector: "job-name==" + jobName,
	}
	search.Kind = "Pod"

	client := k8client.CoreV1().Pods(nameSpace)
	ctx, cancel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
	defer cancel()
	pods, err := client.List(ctx, search)
	if err != nil || len(pods.Items) < 1 {
		log.Panicf("[%s] unable to find the pod for the job %+v", driver.Service, err)
	}

	alllogs := map[string]map[string]string{}
	messages := []string{}

	for _, pod := range pods.Items {
		cStatuses := pod.Status.InitContainerStatuses
		cStatuses = append(cStatuses, pod.Status.ContainerStatuses...)
		for _, c := range cStatuses {
			switch {
			case c.State.Terminated == nil:
				messages = append(messages, fmt.Sprintf("container %s.%s not terminated", pod.Name, c.Name))
			case c.State.Terminated.ExitCode != 0:
				messages = append(messages, fmt.Sprintf("container %s.%s exit with code %+v", pod.Name, c.Name, c.State.Terminated.ExitCode))
			default:
				messages = append(messages, fmt.Sprintf("container %s.%s completed successfully", pod.Name, c.Name))
			}
		}

		messages = append(messages, ">>>Logs<<<")

		podlogs := map[string]string{}
		for _, container := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
				defer cancel()
				stream, err := client.GetLogs(pod.Name, &corev1.PodLogOptions{Container: container.Name}).Stream(ctx)
				if err != nil {
					podlogs[container.Name] = fmt.Sprintf("Error: unable to fetch the logs from the container %s.%s", pod.Name, container.Name)
					messages = append(messages, podlogs[container.Name])
					log.Warningf("[%s] unable to fetch the logs for the pod %s container %s: %+v", driver.Service, pod.Name, container.Name, err)
				} else {
					defer stream.Close()
					containerlog, err := io.ReadAll(stream)
					if err != nil {
						podlogs[container.Name] = fmt.Sprintf("Error: unable to read the logs from the stream %s.%s", pod.Name, container.Name)
						messages = append(messages, podlogs[container.Name])
						log.Warningf("[%s] unable to read logs from stream for pod %s container %s: %+v", driver.Service, pod.Name, container.Name, err)
					} else {
						podlogs[container.Name] = string(containerlog)
						messages = append(messages, podlogs[container.Name])
					}
				}
			}()
		}
		alllogs[pod.Name] = podlogs
	}
	output := strings.Join(messages, "\n")
	returnMsg := dipper.Message{
		Labels: map[string]string{
			"status": jobStatus,
		},
		Payload: map[string]interface{}{
			"log":    alllogs,
			"output": output,
		},
	}
	if jobStatus != StatusSuccess {
		returnMsg.Labels["reason"] = output
	}

	m.Reply <- returnMsg
}

func getJobStatus(job *batchv1.Job) (string, bool, []string) {
	var (
		jobStatus = StatusSuccess
		completed = false
		reason    = []string{}
	)

	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			jobStatus = StatusFailure
			completed = true
			for _, condition := range job.Status.Conditions {
				reason = append(reason, condition.Reason)
			}

			break
		}
		if (cond.Type == batchv1.JobComplete || cond.Type == batchv1.JobSuccessCriteriaMet) && cond.Status == corev1.ConditionTrue {
			jobStatus = StatusSuccess
			completed = true

			break
		}
		log.Infof("[%s] got condition %+v", driver.Service, cond)
	}

	return jobStatus, completed, reason
}

func returnJobStatus(m *dipper.Message, job *batchv1.Job) bool {
	jobStatus, completed, reason := getJobStatus(job)

	if !completed {
		return false
	}

	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"status": job.Status,
		},
		Labels: map[string]string{
			"status": jobStatus,
			"reason": strings.Join(reason, "\n"),
		},
	}

	return true
}

func waitForJob(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}

	jobName := dipper.MustGetMapDataStr(m.Payload, "job")

	timeout := DefaultJobWaitTimeout
	if timeoutStr, ok := m.Labels["timeout"]; ok {
		timeoutInt, _ := strconv.Atoi(timeoutStr)
		timeout = time.Duration(timeoutInt)
	}

	ctxWatch, cancelWatch := context.WithTimeout(context.Background(), timeout*time.Second)
	defer cancelWatch()
	for EOW := false; !EOW; {
		var job *batchv1.Job
		k8client := prepareKubeConfig(m)
		jobclient := k8client.BatchV1().Jobs(nameSpace)

		watchOption := metav1.ListOptions{
			FieldSelector: "metadata.name==" + jobName,
		}
		watchOption.Kind = "job"

		func() {
			listCtx, cancelList := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
			defer cancelList()
			joblist, err := jobclient.List(listCtx, watchOption)
			if err != nil || len(joblist.Items) == 0 {
				log.Panicf("[%s] job not found [%s]: %+v", driver.Service, jobName, err)
			}
			job = &joblist.Items[0]
			watchOption.ResourceVersion = joblist.ResourceVersion
		}()

		if returnJobStatus(m, job) {
			break
		}

		jobstatus, err := jobclient.Watch(ctxWatch, watchOption)
		if err != nil {
			log.Panicf("[%s] unable to watch the job %+v", driver.Service, err)
		}

		defer jobstatus.Stop()

	loop:
		for {
			select {
			case <-ctxWatch.Done():
				returnJobStatus(m, job)
				EOW = true

				break loop
			case evt := <-jobstatus.ResultChan():
				if evt.Object == nil {
					break loop
				}

				if evt.Type == watch.Error {
					e := evt.Object.(*metav1.Status)
					if e.Code == http.StatusGone {
						log.Warningf("[%s] error from watching channel for job [%s]: %+v", driver.Service, jobName, evt.Object)

						break loop
					} else {
						log.Panicf("[%s] error from watching channel for job [%s]: %+v", driver.Service, jobName, evt.Object)
					}
				}

				job := evt.Object.(*batchv1.Job)
				log.Debugf("[%s] receiving a event when watching for job [%s] %s: %+v", driver.Service, jobName, evt.Type, job.Status)
				if returnJobStatus(m, job) {
					EOW = true

					break loop
				}
			}
		}
	}
}

func getExistingJob(jobSpec *batchv1.Job, jobclient batchv1client.JobInterface) *batchv1.Job {
	uniqID, ok := jobSpec.ObjectMeta.Labels[LabelHoneydipperUniqueIdentifier]
	if !ok {
		return nil
	}

	opt := metav1.ListOptions{
		LabelSelector: LabelHoneydipperUniqueIdentifier + "=" + uniqID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
	defer cancel()

	jobList := dipper.Must(jobclient.List(ctx, opt)).(*batchv1.JobList)

	for _, job := range jobList.Items {
		if job.Status.Active > 0 {
			return &job
		}
	}

	return nil
}

func createJob(m *dipper.Message) {
	k8client := prepareKubeConfig(m)

	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}

	job := constructJob(m, nameSpace, k8client)
	jobclient := k8client.BatchV1().Jobs(nameSpace)
	jobResult := getExistingJob(&job, jobclient)
	if jobResult == nil {
		ctx, cancel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
		defer cancel()
		jobResult, err = jobclient.Create(ctx, &job, metav1.CreateOptions{})
		if err != nil {
			log.Panicf("[%s] failed to create job %+v", driver.Service, err)
		}
	}

	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"metadata": jobResult.ObjectMeta,
			"status":   jobResult.Status,
		},
	}
}

func constructJob(m *dipper.Message, namespace string, client *kubernetes.Clientset) batchv1.Job {
	job := batchv1.Job{}

	if fromCronJob, ok := dipper.GetMapDataStr(m.Payload, "fromCronJob"); ok {
		cronJobNamespace, cronJobName := path.Split(fromCronJob)
		if cronJobNamespace == "" {
			cronJobNamespace = namespace
		}

		v1client := client.BatchV1().CronJobs(cronJobNamespace)
		ctx, cancel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
		defer cancel()
		cronJob, err := v1client.Get(ctx, cronJobName, metav1.GetOptions{})

		switch {
		case errors.IsNotFound(err):
			v1beta1client := client.BatchV1().CronJobs(cronJobNamespace)
			ctx2, cancel2 := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
			defer cancel2()
			cronJobv1beta1 := dipper.Must(v1beta1client.Get(ctx2, cronJobName, metav1.GetOptions{})).(*batchv1.CronJob)

			job.Spec = cronJobv1beta1.Spec.JobTemplate.Spec
		case err != nil:
			log.Panicf("[%s] unable get cronJob information %s: %+v", driver.Service, fromCronJob, err)
		default:
			job.Spec = cronJob.Spec.JobTemplate.Spec
		}
	}

	override := batchv1.Job{}
	source := dipper.MustGetMapData(m.Payload, "job")
	buf, err := yaml.Marshal(source)
	if err != nil {
		log.Panicf("[%s] unable to marshal job manifest %+v", driver.Service, err)
	}
	err = yaml.Unmarshal(buf, &override)
	if err != nil {
		log.Panicf("[%s] invalid job manifest %+v", driver.Service, err)
	}

	if err = mergo.Merge(&job, override, mergo.WithOverride, mergo.WithAppendSlice); err != nil {
		log.Panicf("[%s] unable to merge job %w", driver.Service, err)
	}

	log.Debugf("[%s] source %+v job spec %+v", driver.Service, source, job)

	return job
}

func recycleDeployment(m *dipper.Message) {
	k8client := prepareKubeConfig(m)

	deploymentName, ok := dipper.GetMapDataStr(m.Payload, "deployment")
	log.Infof("[%s] got deploymentName %s", driver.Service, deploymentName)
	if !ok {
		log.Panicf("[%s] deployment is missing in parameters", driver.Service)
	}
	useLabelSelector := strings.Contains(deploymentName, "=")
	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}

	var deployment *appsv1.Deployment
	var labels string

	// to accurately identify the replicaset, we have to retrieve the revision
	// from the deployment
	deploymentclient := k8client.AppsV1().Deployments(nameSpace)
	ctx, cancel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
	defer cancel()
	if useLabelSelector {
		deployments, err := deploymentclient.List(ctx, metav1.ListOptions{LabelSelector: deploymentName})
		if err != nil || len(deployments.Items) == 0 {
			log.Panicf("[%s] unable to find the deployment %s: %+v", driver.Service, deploymentName, err)
		}
		deployment = &deployments.Items[0]
		labels = deploymentName
	} else {
		var err error
		deployment, err = deploymentclient.Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			log.Panicf("[%s] unable to find the deployment %s: %+v", driver.Service, deploymentName, err)
		}

		for k, v := range deployment.Spec.Selector.MatchLabels {
			if len(labels) > 0 {
				labels += ","
			}
			labels += k + "=" + v
		}
	}

	revision := deployment.Annotations["deployment.kubernetes.io/revision"]

	rsclient := k8client.AppsV1().ReplicaSets(nameSpace)
	ctxRsList, cancelRsList := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
	defer cancelRsList()
	rs, err := rsclient.List(ctxRsList, metav1.ListOptions{LabelSelector: labels})
	if err != nil || len(rs.Items) == 0 {
		log.Panicf("[%s] unable to find the replicaset for the deployment %s: %+v", driver.Service, deploymentName, err)
	}

	var rsName string

	// annotations are not supported field selectors for replicaset rsource,
	// so we have to iterate over the list to find the one with correct revision
	for _, currentRs := range rs.Items {
		if currentRs.Annotations["deployment.kubernetes.io/revision"] == revision {
			rsName = currentRs.Name

			break
		}
	}
	if len(rsName) == 0 {
		log.Panicf("[%s] unable to figure out which is current replicaset for %s", driver.Service, deploymentName)
	}

	ctxDel, cancelDel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
	defer cancelDel()
	err = rsclient.Delete(ctxDel, rsName, metav1.DeleteOptions{})
	if err != nil {
		log.Panicf("[%s] failed to recycle replicaset %s: %+v", driver.Service, rsName, err)
	}
	log.Infof("[%s] deployment recycled %s.%s", driver.Service, nameSpace, rsName)
	m.Reply <- dipper.Message{}
}

func prepareKubeConfig(m *dipper.Message) *kubernetes.Clientset {
	if log == nil {
		log = driver.GetLogger()
	}
	m = dipper.DeserializePayload(m)

	source, ok := dipper.GetMapData(m.Payload, "source")
	if !ok {
		log.Panicf("[%s] source is missing in parameters", driver.Service)
	}
	stype, ok := dipper.GetMapDataStr(source, "type")
	if !ok {
		log.Panicf("[%s] source type is missing in parameters", driver.Service)
	}
	log.Debugf("[%s] fetching k8config from source", driver.Service)
	var kubeConfig *rest.Config
	switch stype {
	case "gcloud-gke":
		kubeConfig = getGKEConfig(source.(map[string]interface{}))
	case "local":
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			log.Panicf("[%s] unable to load default account for kubernetes %+v", driver.Service, err)
		}
	default:
		log.Panicf("[%s] unsupported kubernetes source type: %s", driver.Service, stype)
	}
	if kubeConfig == nil {
		log.Panicf("[%s] unable to get kubeconfig", driver.Service)
	}

	k8client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Panicf("[%s] unable to create k8 client: %+v", driver.Service, err)
	}

	return k8client
}

func getGKEConfig(cfg map[string]interface{}) *rest.Config {
	retbytes, err := driver.Call("driver:gcloud-gke", "getKubeCfg", cfg)
	if err != nil {
		log.Panicf("[%s] failed call gcloud to get kubeconfig %+v", driver.Service, err)
	}

	ret := dipper.DeserializeContent(retbytes)

	host, _ := dipper.GetMapDataStr(ret, "Host")
	token, _ := dipper.GetMapDataStr(ret, "Token")
	cacert, _ := dipper.GetMapDataStr(ret, "CACert")
	useDNS, _ := dipper.GetMapDataBool(ret, "useDNS")

	k8cfg := &rest.Config{
		Host:        host,
		BearerToken: token,
	}

	if useDNS {
		// GKE DNS based control plane access forcing https. Ignore master auth CA so
		// system CAs can be used instead.
		if !strings.HasPrefix(k8cfg.Host, "https://") {
			k8cfg.Host = "https://" + k8cfg.Host
		}
	} else {
		cadata, _ := base64.StdEncoding.DecodeString(cacert)
		k8cfg.CAData = cadata
	}

	return k8cfg
}

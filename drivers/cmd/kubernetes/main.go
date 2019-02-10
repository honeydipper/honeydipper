// Package kubernetes enables Honeydipper to interact with Kubernete clusters.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/ghodss/yaml"
	"github.com/honeyscience/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// DefaultNamespace is the name of the default space in kubernetes cluster
const DefaultNamespace string = "default"

var log *logging.Logger
var err error

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
	driver.CommandProvider.Commands["recycleDeployment"] = recycleDeployment
	driver.CommandProvider.Commands["createJob"] = createJob
	driver.CommandProvider.Commands["waitForJob"] = waitForJob
	driver.CommandProvider.Commands["getJobLog"] = getJobLog
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func getJobLog(m *dipper.Message) {
	k8client := prepareKubeConfig(m)

	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}
	jobName := dipper.MustGetMapDataStr(m.Payload, "job")
	search := metav1.ListOptions{
		LabelSelector: "job-name==" + jobName,
	}
	search.Kind = "Pod"

	client := k8client.CoreV1().Pods(nameSpace)
	pods, err := client.List(search)
	if err != nil || len(pods.Items) < 1 {
		log.Panicf("[%s] unable to find the pod for the job %+v", driver.Service, err)
	}

	alllogs := map[string]map[string]string{}
	for _, pod := range pods.Items {
		podlogs := map[string]string{}
		for _, container := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
			stream, err := client.GetLogs(pod.Name, &corev1.PodLogOptions{Container: container.Name}).Stream()
			if err != nil {
				podlogs[container.Name] = "Error: unable to fetch the logs from the container"
				log.Warningf("[%s] unable to fetch the logs for the pod %s container %s: %+v", driver.Service, pod.Name, container.Name, err)
			} else {
				func(stream io.ReadCloser) {
					defer stream.Close()
					containerlog, err := ioutil.ReadAll(stream)
					if err != nil {
						podlogs[container.Name] = "Error: unable to read the logs from the stream"
						log.Warningf("[%s] unable to read logs from stream for pod %s container %s: %+v", driver.Service, pod.Name, container.Name, err)
					} else {
						podlogs[container.Name] = string(containerlog)
					}
				}(stream)
			}
		}
		alllogs[pod.Name] = podlogs
	}

	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"log": alllogs,
		},
	}

}

func waitForJob(m *dipper.Message) {
	k8client := prepareKubeConfig(m)

	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}

	jobName := dipper.MustGetMapDataStr(m.Payload, "job")
	timeout := 10
	timeoutStr, ok := dipper.GetMapDataStr(m.Payload, "timeout")
	if ok {
		timeout, _ = strconv.Atoi(timeoutStr)
	}

	jobclient := k8client.BatchV1().Jobs(nameSpace)
	watchOption := metav1.ListOptions{
		FieldSelector: "metadata.name==" + jobName,
	}
	watchOption.Kind = "Job"
	jobstatus, err := jobclient.Watch(watchOption)
	if err != nil {
		log.Panicf("[%s] unable to watch the job %+v", driver.Service, err)
	}

	finalStatus := make(chan dipper.Message, 1)
	go func() {
		for evt := range jobstatus.ResultChan() {
			if evt.Type == "ADDED" || evt.Type == "MODIFIED" {
				job := evt.Object.(*batchv1.Job)
				if len(job.Status.Conditions) > 0 && job.Status.Active == 0 {
					finalStatus <- dipper.Message{
						Payload: map[string]interface{}{
							"status": job.Status,
						},
					}
					break
				}
			}
		}
	}()

	m.Reply <- dipper.Message{
		Labels: map[string]string{
			"no-timeout": "yes",
		},
	} // suppress timeout control

	select {
	case msg := <-finalStatus:
		m.Reply <- msg
		jobstatus.Stop()
	case <-time.After(time.Duration(timeout) * time.Second):
		jobstatus.Stop()
		m.Reply <- dipper.Message{
			Labels: map[string]string{
				"error": "time out",
			},
		}
	}
}

func createJob(m *dipper.Message) {
	k8client := prepareKubeConfig(m)

	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}

	buf, err := yaml.Marshal(dipper.MustGetMapData(m.Payload, "job"))
	if err != nil {
		log.Panicf("[%s] unable to marshal job manifest %+v", driver.Service, err)
	}

	jobSpec := batchv1.Job{}
	err = yaml.Unmarshal(buf, &jobSpec)
	if err != nil {
		log.Panicf("[%s] invalid job manifest %+v", driver.Service, err)
	}
	log.Debugf("[%s] source %+v job spec %+v", driver.Service, dipper.MustGetMapData(m.Payload, "job"), jobSpec)

	jobclient := k8client.BatchV1().Jobs(nameSpace)
	jobResult, err := jobclient.Create(&jobSpec)
	if err != nil {
		log.Panicf("[%s] failed to create job %+v", driver.Service, err)
	}

	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"metadata": jobResult.ObjectMeta,
			"status":   jobResult.Status,
		},
	}
}

func recycleDeployment(m *dipper.Message) {
	k8client := prepareKubeConfig(m)

	deploymentName, ok := dipper.GetMapDataStr(m.Payload, "deployment")
	log.Infof("[%s] got deploymentName %s", driver.Service, deploymentName)
	if !ok {
		log.Panicf("[%s] deployment is missing in parameters", driver.Service)
	}
	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}

	rsclient := k8client.AppsV1().ReplicaSets(nameSpace)
	rs, err := rsclient.List(metav1.ListOptions{LabelSelector: deploymentName})
	if err != nil || len(rs.Items) == 0 {
		log.Panicf("[%s] unable to find the replicaset for the deployment %+v", driver.Service, err)
	}
	rsName := rs.Items[0].Name
	err = rsclient.Delete(rsName, &metav1.DeleteOptions{})
	if err != nil {
		log.Panicf("[%s] failed to recycle replicaset %+v", driver.Service, err)
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
		log.Panicf("[%s] unable to create k8 client", driver.Service)
	}
	return k8client
}

func getGKEConfig(cfg map[string]interface{}) *rest.Config {
	retbytes, err := driver.RPCCall("driver:gcloud-gke", "getKubeCfg", cfg)
	if err != nil {
		log.Panicf("[%s] failed call gcloud to get kubeconfig %+v", driver.Service, err)
	}

	ret := dipper.DeserializeContent(retbytes)

	host, _ := dipper.GetMapDataStr(ret, "Host")
	token, _ := dipper.GetMapDataStr(ret, "Token")
	cacert, _ := dipper.GetMapDataStr(ret, "CACert")

	cadata, _ := base64.StdEncoding.DecodeString(cacert)

	k8cfg := &rest.Config{
		Host:        host,
		BearerToken: token,
	}
	k8cfg.CAData = cadata

	return k8cfg
}

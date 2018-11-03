package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
)

var log = dipper.GetLogger("kubernetes")

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Println("    This driver supports services including receiver, workflow, operator etc")
		fmt.Println("  This program provides honeydipper with capability of interacting with kuberntes")
	}
}

var driver *dipper.Driver

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "kubernetes")
	//if driver.Service == "operator" {
	driver.MessageHandlers["execute:recycleDeployment"] = recycleDeployment
	//}
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func recycleDeployment(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	deploymentName, ok := dipper.GetMapDataStr(m.Payload, "param.deployment")
	log.Infof("[%s] got deploymentName %s", driver.Service, deploymentName)
	if !ok {
		log.Panicf("[%s] deployment is missing in parameters", driver.Service)
	}
	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "param.namespace")
	if !ok {
		nameSpace = "default"
	}
	source, ok := dipper.GetMapData(m.Payload, "param.source")
	if !ok {
		log.Panicf("[%s] source is missing in parameters", driver.Service)
	}
	stype, ok := dipper.GetMapDataStr(source, "type")
	if !ok {
		log.Panicf("[%s] source type is missing in parameters", driver.Service)
	}
	log.Debugf("[%s] fetching k8config from source", driver.Service)
	var kubeConfig *rest.Config
	if stype == "gke" {
		kubeConfig = getGKEConfig(source.(map[string]interface{}))
	} else {
		log.Panicf("[%s] unsupported kubernetes source type: %s", driver.Service, stype)
	}
	if kubeConfig == nil {
		log.Panicf("[%s] unable to get kubeconfig", driver.Service)
	}

	k8client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Panicf("[%s] unable to create k8 client", driver.Service)
	}

	rsclient := k8client.AppsV1().ReplicaSets(nameSpace)
	rs, err := rsclient.List(metav1.ListOptions{LabelSelector: "app=" + deploymentName})
	if err != nil || len(rs.Items) == 0 {
		log.Panicf("[%s] unable to find the replicaset for the deployment %+v", driver.Service, err)
	}
	rsName := rs.Items[0].Name
	err = rsclient.Delete(rsName, &metav1.DeleteOptions{})
	if err != nil {
		log.Panicf("[%s] failed to recycle replicaset %+v", driver.Service, err)
	}
	log.Infof("[%s] deployment recycled %s.%s", driver.Service, nameSpace, rsName)
}

func getGKEConfig(cfg map[string]interface{}) *rest.Config {
	retbytes, err := driver.RPCCall("driver:gcloud.getKubeCfg", cfg)
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

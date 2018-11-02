package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"os"
)

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
	log.Printf("[%s-%s] got deploymentName %s", driver.Service, driver.Name, deploymentName)
	if !ok {
		log.Panicf("[%s-%s] deployment is missing in parameters", driver.Service, driver.Name)
	}
	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "param.namespace")
	if !ok {
		nameSpace = "default"
	}
	source, ok := dipper.GetMapData(m.Payload, "param.source")
	if !ok {
		log.Panicf("[%s-%s] source is missing in parameters", driver.Service, driver.Name)
	}
	stype, ok := dipper.GetMapDataStr(source, "type")
	if !ok {
		log.Panicf("[%s-%s] source type is missing in parameters", driver.Service, driver.Name)
	}
	log.Printf("[%s-%s] fetching k8config from source %+v", driver.Service, driver.Name, source)
	var kubeConfig *rest.Config
	if stype == "gke" {
		kubeConfig = getGKEConfig(source.(map[string]interface{}))
	} else {
		log.Panicf("[%s-%s] unsupported kubernetes source type: %s", driver.Service, driver.Name, stype)
	}
	if kubeConfig == nil {
		log.Panicf("[%s-%s] unable to get kubeconfig", driver.Service, driver.Name)
	}

	k8client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Panicf("[%s-%s] unable to create k8 client", driver.Service, driver.Name)
	}

	rsclient := k8client.AppsV1().ReplicaSets(nameSpace)
	rs, err := rsclient.List(metav1.ListOptions{LabelSelector: "app=" + deploymentName})
	if err != nil || len(rs.Items) == 0 {
		log.Panicf("[%s-%s] unable to find the replicaset for the deployment %+v", driver.Service, driver.Name, err)
	}
	rsName := rs.Items[0].Name
	err = rsclient.Delete(rsName, &metav1.DeleteOptions{})
	if err != nil {
		log.Panicf("[%s-%s] failed to recycle replicaset %+v", driver.Service, driver.Name, err)
	}
	log.Printf("[%s-%s] deployment recycled %s.%s", driver.Service, driver.Name, nameSpace, rsName)
}

func getGKEConfig(cfg map[string]interface{}) *rest.Config {
	retbytes, err := driver.RPCCall("driver:gcloud.getKubeCfg", cfg)
	if err != nil {
		log.Panicf("[%s-%s] failed call gcloud to get kubeconfig %+v", driver.Service, driver.Name, err)
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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/dataflow/v1b3"
	"os"
	"strconv"
	"time"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of interacting with gcloud dataflow")
	}
}

var driver *dipper.Driver

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "gcloud-dataflow")
	driver.CommandProvider.Commands["createJob"] = createJob
	driver.CommandProvider.Commands["waitForJob"] = waitForJob
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func createJob(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, ok := dipper.GetMapDataStr(params, "service_account")
	if !ok {
		panic(errors.New("service_account required"))
	}
	project, ok := dipper.GetMapDataStr(params, "project")
	if !ok {
		panic(errors.New("project required"))
	}
	regional := false
	if regionalData, ok := dipper.GetMapData(params, "regional"); ok {
		regional = regionalData.(bool)
	}
	location, ok := dipper.GetMapDataStr(params, "location")
	if !regional && !ok {
		panic(errors.New("location required for location based dataflow job"))
	}
	job, ok := dipper.GetMapData(params, "job")
	if !ok {
		panic(errors.New("job spec required"))
	}
	var jobSpec dataflow.CreateJobFromTemplateRequest
	err := mapstructure.Decode(job, &jobSpec)
	if err != nil {
		panic(err)
	}

	conf, err := google.JWTConfigFromJSON([]byte(serviceAccountBytes), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		panic(errors.New("invalid service account"))
	}
	dataflowService, err := dataflow.New(conf.Client(oauth2.NoContext))
	if err != nil {
		panic(errors.New("unable to create gcloud client"))
	}
	execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
	var result *dataflow.Job
	func() {
		defer cancel()
		if regional {
			result, err = dataflowService.Projects.Templates.Create(project, &jobSpec).Context(execContext).Do()
		} else {
			result, err = dataflowService.Projects.Locations.Templates.Create(project, location, &jobSpec).Context(execContext).Do()
		}
	}()
	if err != nil {
		panic(errors.New("failed to create dataflow job in gcloud"))
	}
	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"job": *result,
		},
	}
}

func waitForJob(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, ok := dipper.GetMapDataStr(params, "service_account")
	if !ok {
		panic(errors.New("service_account required"))
	}
	project, ok := dipper.GetMapDataStr(params, "project")
	if !ok {
		panic(errors.New("project required"))
	}
	regional := false
	if regionalData, ok := dipper.GetMapData(params, "regional"); ok {
		regional = regionalData.(bool)
	}
	location, ok := dipper.GetMapDataStr(params, "location")
	if !regional && !ok {
		panic(errors.New("location required for location based dataflow job"))
	}
	jobID, ok := dipper.GetMapDataStr(params, "jobID")
	if !ok {
		panic(errors.New("jobID required"))
	}
	timeout := 1800
	timeoutStr, ok := dipper.GetMapDataStr(msg.Payload, "timeout")
	if ok {
		timeout, _ = strconv.Atoi(timeoutStr)
	}
	interval := 10
	intervalStr, ok := dipper.GetMapDataStr(msg.Payload, "interval")
	if ok {
		interval, _ = strconv.Atoi(intervalStr)
	}

	conf, err := google.JWTConfigFromJSON([]byte(serviceAccountBytes), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		panic(errors.New("invalid service account"))
	}
	dataflowService, err := dataflow.New(conf.Client(oauth2.NoContext))
	if err != nil {
		panic(errors.New("unable to create gcloud client"))
	}

	finalStatus := make(chan dipper.Message, 1)
	expired := false
	go func() {
		terminatedStates := map[string]bool{
			"JOB_STATE_DONE":      true,
			"JOB_STATE_FAILED":    true,
			"JOB_STATE_CANCELLED": true,
			"JOB_STATE_UPDATED":   true,
			"JOB_STATE_DRAINED":   true,
		}

		for !expired {
			var result *dataflow.Job
			execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
			func() {
				defer cancel()
				if regional {
					result, err = dataflowService.Projects.Jobs.Get(project, jobID).Context(execContext).Do()
				} else {
					result, err = dataflowService.Projects.Locations.Jobs.Get(project, location, jobID).Context(execContext).Do()
				}
			}()
			if err != nil {
				finalStatus <- dipper.Message{
					Labels: map[string]string{
						"error": "failed to call polling method",
					},
				}
				break
			}

			if _, ok := terminatedStates[result.CurrentState]; ok {
				finalStatus <- dipper.Message{
					Payload: map[string]interface{}{
						"job": *result,
					},
				}
				break
			}

			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()

	msg.Reply <- dipper.Message{
		Labels: map[string]string{
			"no-timeout": "yes",
		},
	} // suppress timeout control

	select {
	case m := <-finalStatus:
		msg.Reply <- m
	case <-time.After(time.Duration(timeout) * time.Second):
		expired = true
		panic(errors.New("Time out"))
	}
}

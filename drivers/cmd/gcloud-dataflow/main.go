// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package gcloud-dataflow enables Honeydipper to create and wait for dataflow jobs.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/oauth2/google"
	dataflow "google.golang.org/api/dataflow/v1b3"
	"google.golang.org/api/googleapi"
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program provides honeydipper with capability of interacting with gcloud dataflow")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "gcloud-dataflow")
	driver.CommandProvider.Commands["createJob"] = createJob
	driver.CommandProvider.Commands["waitForJob"] = waitForJob
	driver.CommandProvider.Commands["getJob"] = getJob
	driver.CommandProvider.Commands["findJobByName"] = findJobByName
	driver.CommandProvider.Commands["updateJob"] = updateJob
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func getDataflowService(serviceAccountBytes string) *dataflow.Service {
	var (
		client *http.Client
		err    error
	)
	if len(serviceAccountBytes) > 0 {
		conf, err := google.JWTConfigFromJSON([]byte(serviceAccountBytes), "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			panic(err)
		}
		client = conf.Client(context.Background())
	} else {
		client, err = google.DefaultClient(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
	}
	if err != nil {
		panic(err)
	}

	dataflowService, err := dataflow.New(client)
	if err != nil {
		panic(err)
	}
	return dataflowService
}

func getCommonParams(params interface{}) (string, string, string) {
	serviceAccountBytes, _ := dipper.GetMapDataStr(params, "service_account")
	project, ok := dipper.GetMapDataStr(params, "project")
	if !ok {
		panic(errors.New("project required"))
	}
	location, ok := dipper.GetMapDataStr(params, "location")
	if ok {
		suffix := location[len(location)-2:]
		if suffix >= "-a" && suffix <= "-z" {
			location = location[:len(location)-2]
		}
	}
	return serviceAccountBytes, project, location
}

func createJob(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, location := getCommonParams(params)

	job, ok := dipper.GetMapData(params, "job")
	if !ok {
		panic(errors.New("job spec required"))
	}
	var jobSpec dataflow.CreateJobFromTemplateRequest
	err := mapstructure.Decode(job, &jobSpec)
	if err != nil {
		panic(err)
	}

	var dataflowService = getDataflowService(serviceAccountBytes)

	execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
	var result *dataflow.Job
	func() {
		defer cancel()
		if len(location) == 0 {
			result, err = dataflowService.Projects.Templates.Create(project, &jobSpec).Context(execContext).Do()
		} else {
			result, err = dataflowService.Projects.Locations.Templates.Create(project, location, &jobSpec).Context(execContext).Do()
		}
	}()
	if err != nil {
		panic(err)
	}
	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"job": *result,
		},
	}
}

func getJob(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, location := getCommonParams(params)

	jobID, ok := dipper.GetMapDataStr(params, "jobID")
	if !ok {
		panic(errors.New("jobID required"))
	}

	var fieldList []googleapi.Field
	if fields, ok := dipper.GetMapData(params, "fields"); ok {
		for _, v := range fields.([]interface{}) {
			fieldList = append(fieldList, v.(googleapi.Field))
		}
	}

	var dataflowService = getDataflowService(serviceAccountBytes)

	var (
		result *dataflow.Job
		err    error
	)
	execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
	func() {
		defer cancel()
		if len(location) == 0 {
			getCall := dataflowService.Projects.Jobs.Get(project, jobID)
			if len(fieldList) > 0 {
				getCall = getCall.Fields(fieldList...)
			}
			result, err = getCall.Context(execContext).Do()
		} else {
			getCall := dataflowService.Projects.Locations.Jobs.Get(project, location, jobID)
			if len(fieldList) > 0 {
				getCall = getCall.Fields(fieldList...)
			}
			result, err = getCall.Context(execContext).Do()
		}
	}()
	if err != nil {
		panic(err)
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"job": *result,
		},
	}
}

func findJobByName(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, location := getCommonParams(params)

	jobName, ok := dipper.GetMapDataStr(params, "name")
	if !ok {
		panic(errors.New("name required"))
	}

	var dataflowService = getDataflowService(serviceAccountBytes)

	listJobCall := dataflowService.Projects.Jobs.List(project)
	fieldList := []googleapi.Field{
		"nextPageToken",
		"jobs(id,name,currentState)",
	}
	listJobCall = listJobCall.Fields(fieldList...)
	listJobCall = listJobCall.Filter("ACTIVE")
	if len(location) > 0 {
		listJobCall = listJobCall.Location(location)
	}

	var (
		result *dataflow.ListJobsResponse
		err    error
		job    *dataflow.Job
	)
	for job == nil {
		execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
		func() {
			defer cancel()
			result, err = listJobCall.Context(execContext).Do()
		}()

		if err != nil {
			panic(err)
		}

		if len(result.Jobs) > 0 {
			for _, j := range result.Jobs {
				if j.Name == jobName {
					job = j
					break
				}
			}
		}

		if len(result.NextPageToken) > 0 {
			listJobCall.PageToken(result.NextPageToken)
		} else {
			break
		}
	}

	if job != nil {
		msg.Reply <- dipper.Message{
			Payload: map[string]interface{}{
				"job": *job,
			},
		}
	} else {
		panic(errors.New("job not found"))
	}
}

func waitForJob(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, location := getCommonParams(params)

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

	var dataflowService = getDataflowService(serviceAccountBytes)

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
			var (
				result *dataflow.Job
				err    error
			)
			execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
			func() {
				defer cancel()
				if len(location) == 0 {
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
		panic(errors.New("time out"))
	}
}

func updateJob(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, location := getCommonParams(params)

	job, ok := dipper.GetMapData(params, "jobSpec")
	if !ok {
		panic(errors.New("job spec required"))
	}
	var jobSpec dataflow.Job
	err := mapstructure.Decode(job, &jobSpec)
	if err != nil {
		panic(err)
	}
	jobID := dipper.MustGetMapDataStr(params, "jobID")

	var dataflowService = getDataflowService(serviceAccountBytes)

	execContext, cancel := context.WithTimeout(context.Background(), time.Second*10)
	var result *dataflow.Job
	func() {
		defer cancel()
		if len(location) == 0 {
			result, err = dataflowService.Projects.Jobs.Update(project, jobID, &jobSpec).Context(execContext).Do()
		} else {
			result, err = dataflowService.Projects.Locations.Jobs.Update(project, location, jobID, &jobSpec).Context(execContext).Do()
		}
	}()
	if err != nil {
		panic(err)
	}
	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"job": *result,
		},
	}
}

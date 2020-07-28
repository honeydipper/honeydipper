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
	"os"
	"strconv"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	dataflow "google.golang.org/api/dataflow/v1b3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
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
	driver.Commands["createJob"] = createJob
	driver.Commands["waitForJob"] = waitForJob
	driver.Commands["getJob"] = getJob
	driver.Commands["findJobByName"] = findJobByName
	driver.Commands["updateJob"] = updateJob
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func getDataflowService(serviceAccountBytes string) *dataflow.Service {
	var (
		dataflowService *dataflow.Service
		err             error
	)
	if len(serviceAccountBytes) > 0 {
		dataflowService, err = dataflow.NewService(context.Background(), option.WithCredentialsJSON([]byte(serviceAccountBytes)))
	} else {
		dataflowService, err = dataflow.NewService(context.Background())
	}
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
	dipper.PanicError(mapstructure.Decode(job, &jobSpec))

	var dataflowService = getDataflowService(serviceAccountBytes)

	result := getExistingJob(project, location, jobSpec.JobName, dataflowService)
	if result == nil {
		execContext, cancel := context.WithTimeout(context.Background(), time.Second*driver.APITimeout)
		func() {
			defer cancel()
			if len(location) == 0 {
				result = dipper.Must(
					dataflowService.Projects.Templates.Create(project, &jobSpec).Context(execContext).Do(),
				).(*dataflow.Job)
			} else {
				result = dipper.Must(
					dataflowService.Projects.Locations.Templates.Create(project, location, &jobSpec).Context(execContext).Do(),
				).(*dataflow.Job)
			}
		}()
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
	execContext, cancel := context.WithTimeout(context.Background(), time.Second*driver.APITimeout)
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

func getExistingJob(project, location, jobName string, dataflowService *dataflow.Service) *dataflow.Job {
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
		job    *dataflow.Job
	)

found:
	for job == nil {
		execContext, cancel := context.WithTimeout(context.Background(), time.Second*driver.APITimeout)
		func() {
			defer cancel()
			result = dipper.Must(listJobCall.Context(execContext).Do()).(*dataflow.ListJobsResponse)
		}()

		if len(result.Jobs) > 0 {
			for _, j := range result.Jobs {
				if j.Name == jobName {
					job = j
					break found
				}
			}
		}

		if len(result.NextPageToken) > 0 {
			listJobCall.PageToken(result.NextPageToken)
		} else {
			break
		}
	}

	return job
}

func findJobByName(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, location := getCommonParams(params)
	jobName, ok := dipper.GetMapDataStr(params, "name")
	if !ok {
		panic(errors.New("missing name"))
	}
	dataflowService := getDataflowService(serviceAccountBytes)

	job := getExistingJob(project, location, jobName, dataflowService)

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
	interval := 10
	intervalStr, ok := dipper.GetMapDataStr(msg.Payload, "interval")
	if ok {
		interval, _ = strconv.Atoi(intervalStr)
	}
	timeout := time.Duration(1800)
	timeoutStr, ok := msg.Labels["timeout"]
	if ok {
		timeoutInt, _ := strconv.Atoi(timeoutStr)
		timeout = time.Duration(timeoutInt)
	}

	var dataflowService = getDataflowService(serviceAccountBytes)

	terminatedStates := map[string]string{
		"JOB_STATE_DONE":      "success",
		"JOB_STATE_FAILED":    "failure",
		"JOB_STATE_CANCELLED": "failure",
		"JOB_STATE_UPDATED":   "success",
		"JOB_STATE_DRAINED":   "success",
	}

	expired := time.After(timeout * time.Second)

	var (
		result *dataflow.Job
		err    error
	)

loop:
	for {
		select {
		case <-expired:
			break loop
		default:
			func() {
				execContext, cancel := context.WithTimeout(context.Background(), time.Second*driver.APITimeout)
				defer cancel()
				if len(location) == 0 {
					result, err = dataflowService.Projects.Jobs.Get(project, jobID).Context(execContext).Do()
				} else {
					result, err = dataflowService.Projects.Locations.Jobs.Get(project, location, jobID).Context(execContext).Do()
				}
			}()

			if err != nil {
				msg.Reply <- dipper.Message{
					Labels: map[string]string{
						"error": fmt.Sprintf("failed to call polling method: %+v", err),
					},
				}
				break loop
			}

			if status, ok := terminatedStates[result.CurrentState]; ok {
				msg.Reply <- dipper.Message{
					Payload: map[string]interface{}{
						"job": *result,
					},
					Labels: map[string]string{
						"status": status,
						"reason": result.CurrentState,
					},
				}
				break loop
			}
			time.Sleep(time.Duration(interval) * time.Second)
		}
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

	execContext, cancel := context.WithTimeout(context.Background(), time.Second*driver.APITimeout)
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

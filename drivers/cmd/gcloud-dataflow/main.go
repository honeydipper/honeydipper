// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package gcloud-dataflow enables Honeydipper to create and wait for dataflow jobs.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/mitchellh/mapstructure"
	dataflow "google.golang.org/api/dataflow/v1b3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

var (
	// ErrMissingProject means missing project.
	ErrMissingProject = errors.New("project required")
	// ErrMissingJobSpec means missing location.
	ErrMissingJobSpec = errors.New("job spec required")
	// ErrMissingJobID means missing jobid.
	ErrMissingJobID = errors.New("jobid required")
	// ErrMissingName means missing name.
	ErrMissingName = errors.New("name required")
	// ErrJobNotFound means job not found.
	ErrJobNotFound = errors.New("job not found")
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
	driver.Commands["waitForJob|interruptible"] = waitForJob
	driver.DefaultTimeout["waitForJob"] = "30m"
	driver.Commands["getJob"] = getJob
	driver.Commands["findJobByName"] = findJobByName
	driver.Commands["updateJob"] = updateJob
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func getDataflowService(ctx context.Context, serviceAccountBytes string) *dataflow.Service {
	var dataflowService *dataflow.Service
	if len(serviceAccountBytes) > 0 {
		dataflowService = dipper.Must(dataflow.NewService(ctx, option.WithAuthCredentialsJSON(
			option.ServiceAccount, []byte(serviceAccountBytes)))).(*dataflow.Service)
	} else {
		dataflowService = dipper.Must(dataflow.NewService(ctx)).(*dataflow.Service)
	}

	return dataflowService
}

func getCommonParams(params interface{}) (string, string, string) {
	serviceAccountBytes, _ := dipper.GetMapDataStr(params, "service_account")
	project, ok := dipper.GetMapDataStr(params, "project")
	if !ok {
		panic(ErrMissingProject)
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
		panic(ErrMissingJobSpec)
	}
	var jobSpec dataflow.CreateJobFromTemplateRequest
	dipper.Must(mapstructure.Decode(job, &jobSpec))

	ctx, cancel := driver.GetContext(msg)
	defer cancel()
	dataflowService := getDataflowService(ctx, serviceAccountBytes)

	result := getExistingJob(ctx, project, location, "^"+jobSpec.JobName+"$", dataflowService)
	if result == nil {
		if len(location) == 0 {
			result = dipper.Must(
				dataflowService.Projects.Templates.Create(project, &jobSpec).Context(ctx).Do(),
			).(*dataflow.Job)
		} else {
			result = dipper.Must(
				dataflowService.Projects.Locations.Templates.Create(project, location, &jobSpec).Context(ctx).Do(),
			).(*dataflow.Job)
		}
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
		panic(ErrMissingJobID)
	}

	var fieldList []googleapi.Field
	if fields, ok := dipper.GetMapData(params, "fields"); ok {
		for _, v := range fields.([]interface{}) {
			fieldList = append(fieldList, v.(googleapi.Field))
		}
	}

	ctx, cancel := driver.GetContext(msg)
	defer cancel()
	dataflowService := getDataflowService(ctx, serviceAccountBytes)

	var result any
	if len(location) == 0 {
		getCall := dataflowService.Projects.Jobs.Get(project, jobID)
		if len(fieldList) > 0 {
			getCall = getCall.Fields(fieldList...)
		}
		result = dipper.Must(getCall.Context(ctx).Do())
	} else {
		getCall := dataflowService.Projects.Locations.Jobs.Get(project, location, jobID)
		if len(fieldList) > 0 {
			getCall = getCall.Fields(fieldList...)
		}
		result = dipper.Must(getCall.Context(ctx).Do())
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"job": result,
		},
	}
}

func getExistingJob(ctx context.Context, project, location, jobName string, dataflowService *dataflow.Service) *dataflow.Job {
	pattern := regexp.MustCompile(jobName)

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
		result = dipper.Must(listJobCall.Context(ctx).Do()).(*dataflow.ListJobsResponse)

		if len(result.Jobs) > 0 {
			for _, j := range result.Jobs {
				if pattern.Match([]byte(j.Name)) {
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
		panic(ErrMissingName)
	}

	ctx, cancel := driver.GetContext(msg)
	defer cancel()
	dataflowService := getDataflowService(ctx, serviceAccountBytes)

	job := getExistingJob(ctx, project, location, jobName, dataflowService)

	if job != nil {
		msg.Reply <- dipper.Message{
			Payload: map[string]interface{}{
				"job": *job,
			},
		}
	} else {
		panic(ErrJobNotFound)
	}
}

func waitForJob(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, location := getCommonParams(params)

	jobID, ok := dipper.GetMapDataStr(params, "jobID")
	if !ok {
		panic(ErrMissingJobID)
	}
	interval := 10
	intervalStr, ok := dipper.GetMapDataStr(msg.Payload, "interval")
	if ok {
		interval, _ = strconv.Atoi(intervalStr)
	}

	ctx, cancel := driver.GetContext(msg)
	defer cancel()
	dataflowService := getDataflowService(ctx, serviceAccountBytes)

	terminatedStates := map[string]string{
		"JOB_STATE_DONE":      "success",
		"JOB_STATE_FAILED":    "failure",
		"JOB_STATE_CANCELLED": "failure",
		"JOB_STATE_UPDATED":   "success",
		"JOB_STATE_DRAINED":   "success",
	}

	var result *dataflow.Job

	for {
		if len(location) == 0 {
			result = dipper.Must(dataflowService.Projects.Jobs.Get(project, jobID).Context(ctx).Do()).(*dataflow.Job)
		} else {
			result = dipper.Must(dataflowService.Projects.Locations.Jobs.Get(project, location, jobID).Context(ctx).Do()).(*dataflow.Job)
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

			return
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func updateJob(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, location := getCommonParams(params)

	jobID := dipper.MustGetMapDataStr(params, "jobID")
	job, ok := dipper.GetMapData(params, "jobSpec")
	if !ok {
		panic(ErrMissingJobSpec)
	}
	var jobSpec dataflow.Job
	dipper.Must(mapstructure.Decode(job, &jobSpec))

	ctx, cancel := driver.GetContext(msg)
	defer cancel()
	dataflowService := getDataflowService(ctx, serviceAccountBytes)

	var result *dataflow.Job
	if len(location) == 0 {
		result = dipper.Must(dataflowService.Projects.Jobs.Update(project, jobID, &jobSpec).Context(ctx).Do()).(*dataflow.Job)
	} else {
		result = dipper.Must(dataflowService.Projects.Locations.Jobs.Update(project, location, jobID, &jobSpec).Context(ctx).Do()).(*dataflow.Job)
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"job": *result,
		},
	}
}

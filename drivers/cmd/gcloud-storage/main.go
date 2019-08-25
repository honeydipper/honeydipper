// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package gcloud-storage enables Honeydipper to fetch gcloud storage bucket and file information
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of interacting with gcloud storage\n")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "gcloud-storage")
	driver.CommandProvider.Commands["listBuckets"] = listBuckets
	driver.CommandProvider.Commands["listFiles"] = listFiles
	driver.CommandProvider.Commands["fetchFile"] = fetchFile
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func getStorageClient(serviceAccountBytes string) *storage.Client {
	var (
		client *storage.Client
		err    error
	)
	ctx := context.Background()
	if len(serviceAccountBytes) > 0 {
		clientOption := option.WithCredentialsJSON([]byte(serviceAccountBytes))
		client, err = storage.NewClient(ctx, clientOption)
	} else {
		client, err = storage.NewClient(ctx)
	}
	if err != nil {
		panic(errors.New("unable to create gcloud pubsub client"))
	}
	return client
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

// BucketIterator is an interface for iterate BucketAttrs
type BucketIterator interface {
	Next() (*storage.BucketAttrs, error)
}

// ObjectIterator is an interface for iterate ObjectAttrs
type ObjectIterator interface {
	Next() (*storage.ObjectAttrs, error)
}

func listBuckets(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, project, _ := getCommonParams(params)

	var client = getStorageClient(serviceAccountBytes)

	it := client.Buckets(context.Background(), project)
	listBucketsHelper(msg, it)
}

func listBucketsHelper(msg *dipper.Message, it BucketIterator) {
	var buckets []string

	for {
		battrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}
		buckets = append(buckets, battrs.Name)
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"buckets": buckets,
		},
	}
}

func listFiles(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, _, _ := getCommonParams(params)

	bucket, ok := dipper.GetMapDataStr(params, "bucket")
	if !ok {
		panic(errors.New("bucket spec required"))
	}
	prefix, _ := dipper.GetMapDataStr(params, "prefix")
	delim, _ := dipper.GetMapDataStr(params, "delimiter")

	var client = getStorageClient(serviceAccountBytes)
	query := &storage.Query{
		Prefix:    prefix,
		Delimiter: delim,
	}

	it := client.Bucket(bucket).Objects(context.Background(), query)
	listFilesHelper(msg, it)
}

func listFilesHelper(msg *dipper.Message, it ObjectIterator) {
	files := make([]string, 0)
	prefixes := make([]string, 0)

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}

		if attrs.Prefix != "" {
			prefixes = append(prefixes, attrs.Prefix)
		} else {
			files = append(files, attrs.Name)
		}
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"files":    files,
			"prefixes": prefixes,
		},
	}
}

func fetchFile(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload
	serviceAccountBytes, _, _ := getCommonParams(params)

	bucket, ok := dipper.GetMapDataStr(params, "bucket")
	if !ok {
		panic(errors.New("bucket spec required"))
	}
	fileObj, ok := dipper.GetMapDataStr(params, "fileObject")
	if !ok {
		panic(errors.New("fileObject spec required"))
	}
	fileType, _ := dipper.GetMapDataStr(params, "fileType")

	var client = getStorageClient(serviceAccountBytes)
	ctx := context.Background()
	rc, err := client.Bucket(bucket).Object(fileObj).NewReader(ctx)
	if err != nil {
		panic(err)
	}
	defer rc.Close()

	content, err := ioutil.ReadAll(rc)
	if err != nil {
		panic(err)
	}

	if fileType != "" {
		contentType := http.DetectContentType(content)
		if contentType != fileType {
			panic(errors.New("file content type and specified fileType mismatch"))
		}
	}

	msg.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"content": string(content),
		},
	}
}

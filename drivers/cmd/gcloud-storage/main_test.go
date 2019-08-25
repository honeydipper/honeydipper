// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package main

import (
	"os"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
	mock_storage "github.com/honeydipper/honeydipper/drivers/cmd/gcloud-storage/mocks"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iterator"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
		dipper.Logger = dipper.GetLogger("test", "INFO", logFile, logFile)
	}
	driver = &dipper.Driver{Service: "test"}
	m.Run()
}

func TestListBuckets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBktIt := mock_storage.NewMockBucketIterator(ctrl)
	mockBktIt.EXPECT().Next().Times(1).Return(&storage.BucketAttrs{
		Name: "bucket1",
	}, nil)
	mockBktIt.EXPECT().Next().Times(1).Return(&storage.BucketAttrs{
		Name: "bucket2",
	}, nil)
	mockBktIt.EXPECT().Next().Times(1).Return(nil, iterator.Done)

	msg := &dipper.Message{
		Reply: make(chan dipper.Message, 1),
	}
	go listBucketsHelper(msg, mockBktIt)

	got := <-msg.Reply
	want := dipper.Message{
		Payload: map[string]interface{}{
			"buckets": []string{
				"bucket1",
				"bucket2",
			},
		},
	}
	close(msg.Reply)
	assert.Equalf(t, want, got, "Driver message's Reply Payload mismatch")
}

func TestListFiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockObjIt := mock_storage.NewMockObjectIterator(ctrl)
	mockObjIt.EXPECT().Next().Return(&storage.ObjectAttrs{
		Name:   "file1",
		Prefix: "",
	}, nil)
	mockObjIt.EXPECT().Next().Return(&storage.ObjectAttrs{
		Name:   "path/file2",
		Prefix: "",
	}, nil)
	mockObjIt.EXPECT().Next().Return(nil, iterator.Done)

	msg := &dipper.Message{
		Reply: make(chan dipper.Message, 1),
	}
	go listFilesHelper(msg, mockObjIt)

	got := <-msg.Reply
	want := dipper.Message{
		Payload: map[string]interface{}{
			"files": []string{
				"file1",
				"path/file2",
			},
			"prefixes": []string{},
		},
	}
	close(msg.Reply)
	assert.Equalf(t, want, got, "Driver message's Reply Payload mismatch")
}

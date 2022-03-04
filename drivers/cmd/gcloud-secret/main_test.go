// Copyright 2021 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !integration
// +build !integration

package main

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	mock_driver "github.com/honeydipper/honeydipper/drivers/cmd/gcloud-secret/mock_gcloud-secret"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.Create("test.log")
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}

func TestLookupWithoutName(t *testing.T) {
	driver = dipper.NewDriver(os.Args[1], "secretmanager")
	ctrl := gomock.NewController(t)
	client := mock_driver.NewMockSecretManagerClient(ctrl)
	loadOptions(&dipper.Message{})
	_clientPool.Put(client)
	defer _clientPool.Close()

	// client.EXPECT().Close().Times(1).Return(nil)

	assert.PanicsWithValue(t, ErrSecretNameMissing, func() { lookup(&dipper.Message{}) }, "should panic without the secret name")
}

func TestLookupWithName(t *testing.T) {
	driver = dipper.NewDriver(os.Args[1], "secretmanager")
	ctrl := gomock.NewController(t)
	client := mock_driver.NewMockSecretManagerClient(ctrl)
	loadOptions(&dipper.Message{})
	_clientPool.Put(client)
	defer _clientPool.Close()

	// client.EXPECT().Close().Times(1).Return(nil)
	client.EXPECT().AccessSecretVersion(
		gomock.Any(),
		gomock.Eq(&secretmanagerpb.AccessSecretVersionRequest{
			Name: "projects/myproject/secrets/secretname/versions/latest",
		}),
	).Times(1).Return(
		&secretmanagerpb.AccessSecretVersionResponse{
			Payload: &secretmanagerpb.SecretPayload{
				Data: []byte("plaintext"),
			},
		},
		nil,
	)

	msg := &dipper.Message{
		Payload: []byte("myproject/secretname"),
		Reply:   make(chan dipper.Message, 1),
	}

	assert.NotPanics(t, func() { lookup(msg) }, "should not panic when looking up.")
	select {
	case ret := <-msg.Reply:
		assert.Equal(t, []byte("plaintext"), ret.Payload, "return value should be 'plaintext'.")
	default:
		assert.Fail(t, "should receive plain text in reply chan.")
	}
}

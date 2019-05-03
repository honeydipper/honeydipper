// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		logFile, err := os.Create("test.log")
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
		log = dipper.GetLogger("test", "INFO", logFile, logFile)
	}
	driver = &dipper.Driver{Service: "test"}
	m.Run()
}

func TestExtractEvent(t *testing.T) {
	var eventData map[string]interface{}
	var server *http.Server
	var waitgroup sync.WaitGroup
	hookHandlerTest := func(w http.ResponseWriter, r *http.Request) {
		eventData = extractEventData(w, r)
		w.WriteHeader(http.StatusOK)
		go server.Shutdown(context.Background())
	}
	server = &http.Server{
		Addr:    "127.0.0.1:8999",
		Handler: http.HandlerFunc(hookHandlerTest),
	}
	waitgroup.Add(1)
	go func() {
		defer waitgroup.Done()
		server.ListenAndServe()
	}()
	// without this the client will send request too early and server is not ready
	<-time.After(100 * time.Millisecond)
	resp, err := http.Get("http://127.0.0.1:8999")

	assert.NoError(t, err, "client should not get err")
	assert.NotEmpty(t, resp, "response should not be empty")
	assert.NotNil(t, resp.Body, "response body stream should not be nil")

	resp.Body.Close()
	waitgroup.Wait()

	assert.Containsf(t, eventData, "host", "host is missing in eventData")
	assert.Containsf(t, eventData, "remoteAddr", "remoteAddr is missing in eventData")
	assert.Equalf(t, "127.0.0.1:8999", eventData["host"], "host data mismatch")
	assert.Containsf(t, eventData["remoteAddr"], "127.0.0.1:", "remoteAddr data mismatch")
}

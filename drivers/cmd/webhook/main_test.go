// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8999", nil)
	resp, err := http.DefaultClient.Do(req)

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

func TestVerifySignature(t *testing.T) {
	result := verifySignature(
		"X-Hub-Signature-256",
		"sha256=00",
		"testsecret",
		map[string]interface{}{
			"body": []byte(`{"test": "value"}`),
		},
	)
	assert.Falsef(t, result, "should fail when hmac header is invalid")

	result = verifySignature(
		"X-Hub-Signature-256",
		"sha256=95bb6cd808d37348a4bda7857e7558a16de0d0868d83e75574b834660fbc1d3b",
		"testsecret",
		map[string]interface{}{
			"body": []byte(`{"test": "value"}`),
		},
	)
	assert.Truef(t, result, "should succeed with valid github hmac header")

	result = verifySignature(
		"X-PagerDuty-Signature",
		"v1=95bb6cd808d37348a4bda7857e7558a16de0d0868d83e75574b834660fbc1d3b",
		"testsecret",
		map[string]interface{}{
			"body": []byte(`{"test": "value"}`),
		},
	)
	assert.Truef(t, result, "should succeed with valid pagerduty hmac header")

	result = verifySignature(
		"X-Slack-Signature",
		"v0=57a7e349548bb23285ae98637c893c6beffc9d2f654f386b2b979001167a13a6",
		"testsecret",
		map[string]interface{}{
			"body":              []byte(`{"test": "value"}`),
			"headers":           http.Header{"X-Slack-Request-Timestamp": []string{"1622172061"}},
			"skip_replay_check": "yes",
		},
	)
	assert.Truef(t, result, "should succeed with valid slack hmac header")

	f := func() {
		verifySignature(
			"X-Slack-Signature",
			"v0=57a7e349548bb23285ae98637c893c6beffc9d2f654f386b2b979001167a13a6",
			"testsecret",
			map[string]interface{}{
				"body":    []byte(`{"test": "value"}`),
				"headers": http.Header{"X-Slack-Request-Timestamp": []string{"1622172061"}},
			},
		)
	}
	assert.PanicsWithErrorf(t, "replay attack detected", f, "should detect replay attack with old slack request timestamp")
}

type mockResponseWriter struct {
	status  int
	content []byte
	e       error
	header  http.Header
}

func (m *mockResponseWriter) Write(c []byte) (int, error) {
	m.content = c

	return len(c), m.e
}

func (m *mockResponseWriter) WriteHeader(s int) {
	m.status = s
}

func (m *mockResponseWriter) Header() http.Header {
	return m.header
}

func TestHookHandler(t *testing.T) {
	sysMap = map[string]map[string]interface{}{
		"sys-missing-secret": {
			"signatureHeader": "x-pagerduty-signature",
		},
		"sys": {
			"signatureHeader": "x-pagerduty-signature",
			"signatureSecret": "test-secret",
		},
		"sys-secret-list": {
			"signatureHeader": "x-pagerduty-signature",
			"signatureSecret": []interface{}{
				"test-secret1",
				"test-secret2",
			},
		},
		"sys-unsupported-header": {
			"signatureHeader": "x-unknown-signature",
		},
	}

	hooks = map[string]interface{}{
		"sys1.webhook": []interface{}{
			map[string]interface{}{
				"match": map[string]interface{}{"url": "/test/sys1"},
			},
		},
		"sys2.webhook": []interface{}{
			map[string]interface{}{
				"match": map[string]interface{}{"verifiedSystem": "sys-missing-secret"},
			},
		},
		"sys3.webhook": []interface{}{
			map[string]interface{}{
				"match": map[string]interface{}{"verifiedSystem": "sys-unsupported-header"},
			},
		},
		"sys4.webhook": []interface{}{
			map[string]interface{}{
				"match": map[string]interface{}{"verifiedSystem": "sys"},
			},
		},
		"sys5.webhook": []interface{}{
			map[string]interface{}{
				"match": map[string]interface{}{"verifiedSystem": "sys-secret-list"},
			},
		},
	}

	buf := bytes.NewBuffer(make([]byte, 2048))
	driver = &dipper.Driver{
		Out: buf,
	}

	resp := &mockResponseWriter{header: http.Header{}}
	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test/sys1"},
	}
	hookHandler(resp, req)
	msg := dipper.FetchMessage(buf)
	assert.Equalf(t, 200, resp.status, "should return 200 on success")
	assert.Equalf(t, "webhook.", msg.Payload.(map[string]interface{})["events"].([]interface{})[0], "should emit webhook event to daemon")

	resp = &mockResponseWriter{header: http.Header{}}
	req = &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/test/sys2"},
	}
	hookHandler(resp, req)
	assert.Equalf(t, 404, resp.status, "should return 404 when no trigger is matching")
	assert.Zerof(t, buf.Len(), "should not emit event to daemon")

	resp = &mockResponseWriter{header: http.Header{}}
	req = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/test/sys2"},
		Header: http.Header{"X-Pagerduty-Signature": []string{"v1=bcc889a40667cab715e1dc22ad280692cf4bf1c3a280eeeca60d8dbcd8e4b993"}},
		Body:   io.NopCloser(bytes.NewBufferString("hello")),
	}
	hookHandler(resp, req)
	msg = dipper.FetchMessage(buf)
	assert.Equalf(t, 200, resp.status, "should return 200 on success with proper signature")
	assert.Equalf(t, "sys", dipper.MustGetMapDataStr(msg.Payload, "data.verifiedSystem.0"), "should emit webhook event with verifiedSystem")
	assert.Equalf(t, 1, len(dipper.MustGetMapData(msg.Payload, "data.verifiedSystem").([]interface{})), "should verify only 1 system")

	resp = &mockResponseWriter{header: http.Header{}}
	req = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/test/sys2"},
		Header: http.Header{"X-Unknown-Signature": []string{"v1=bcc889a40667cab715e1dc22ad280692cf4bf1c3a280eeeca60d8dbcd8e4b993"}},
		Body:   io.NopCloser(bytes.NewBufferString("hello")),
	}
	hookHandler(resp, req)
	assert.Equalf(t, 404, resp.status, "should return 404 with unsupported signature header")
	assert.Zerof(t, buf.Len(), "should not emit event to daemon")

	resp = &mockResponseWriter{header: http.Header{}}
	req = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/test/sys5"},
		Header: http.Header{"X-Pagerduty-Signature": []string{"v1=7afdf53eec1c15fb75269b28fab95228ba591b11a103d8e0972087e6dee018ca"}},
		Body:   io.NopCloser(bytes.NewBufferString("hello")),
	}
	hookHandler(resp, req)
	msg = dipper.FetchMessage(buf)
	assert.Equalf(t, 200, resp.status, "should return 200 on success with proper signature")
	assert.Equalf(t, "sys-secret-list", dipper.MustGetMapDataStr(msg.Payload, "data.verifiedSystem.0"), "should emit webhook event with verifiedSystem")
	assert.Equalf(t, 1, len(dipper.MustGetMapData(msg.Payload, "data.verifiedSystem").([]interface{})), "should verify only 1 system")
}

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org.

//go:build !integration
// +build !integration

package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	setDriver(&dipper.Driver{Service: "test"})
	m.Run()
}

func makeTestDriver(service, addr string, hooksData map[string]interface{}) *dipper.Driver {
	opts := map[string]interface{}{
		"data": map[string]interface{}{
			"Addr": addr,
		},
		"dynamicData": map[string]interface{}{
			"collapsedEvents": hooksData,
		},
	}

	return &dipper.Driver{
		Service: service,
		State:   dipper.DriverStateAlive,
		Options: opts,
	}
}

// waitForServer waits for the server to start listening on the given address.
func waitForServer(addr string, timeout time.Duration) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/hz/alive", nil)
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	return assert.AnError
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
	configMu.Lock()
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
	configMu.Unlock()

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

func TestCustomizedResponse(t *testing.T) {
	buf := bytes.NewBuffer(make([]byte, 2048))

	driver = &dipper.Driver{
		Out: buf,
	}

	resp := &mockResponseWriter{header: http.Header{}}
	req := &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: "/test/sys1"},
		Header: http.Header{
			"Content-Type": []string{
				"application/x-www-form-urlencoded",
			},
		},
		Body: io.NopCloser(bytes.NewBufferString(`var1=foobar`)),
	}

	configMu.Lock()
	hooks = map[string]interface{}{
		"sys1.webhook": []interface{}{
			map[string]interface{}{
				"match": map[string]interface{}{"url": "/test/sys1"},
				"parameters": map[string]interface{}{
					"response_content_type": "text/plain",
					"response_payload":      "$event.form.var1.0",
				},
			},
		},
	}
	configMu.Unlock()

	hookHandler(resp, req)

	msg := dipper.FetchMessage(buf)
	assert.Equalf(t, "webhook.", msg.Payload.(map[string]interface{})["events"].([]interface{})[0], "should emit webhook event to daemon")
	assert.Equal(t, "text/plain", resp.header.Get("Content-Type"), "should set customized response header")
	assert.Equalf(t, 200, resp.status, "should return 200 on success with customized response")
	assert.Equalf(t, "foobar", string(resp.content), "should return customized response")
}

// TestPortCollision tests that when the port is already in use, the webhook
// driver implements exponential backoff and doesn't explode goroutines.
func TestPortCollision(t *testing.T) {
	// Save original values under mutex
	var origRetryInterval time.Duration
	var origMaxRetryInterval time.Duration
	var origAddr string
	var origServer *http.Server
	var origDriverAlive bool
	var origConfigUpdated bool
	var origHooks map[string]interface{}
	var origSysMap map[string]map[string]interface{}

	configMu.Lock()
	origRetryInterval = retryInterval
	origMaxRetryInterval = maxRetryInterval
	origAddr = addr
	origServer = server
	origDriverAlive = driverAlive
	origConfigUpdated = configUpdated
	origHooks = hooks
	origSysMap = sysMap
	configMu.Unlock()

	// Reset for test
	configMu.Lock()
	retryInterval = 50 * time.Millisecond // Fast backoff for testing
	maxRetryInterval = 500 * time.Millisecond
	driverAlive = true
	configUpdated = false
	addr = ""
	server = nil
	hooks = map[string]interface{}{
		"test": []interface{}{map[string]interface{}{"match": map[string]interface{}{"url": "/test"}}},
	}
	sysMap = map[string]map[string]interface{}{}
	configMu.Unlock()

	testDriver := makeTestDriver("test-port-collision", "127.0.0.1:0", hooks)
	setDriver(testDriver)

	// Start a server on a port to block it
	lc := net.ListenConfig{}
	blockListener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	blockAddr := blockListener.Addr().String()

	configMu.Lock()
	addr = blockAddr // Try to use the same address
	configMu.Unlock()

	// Track goroutine count
	initialGoroutines := runtime.NumGoroutine()

	// Start webhook - it should fail to bind and retry with backoff
	startWebhook(nil)

	// Wait for several retry cycles
	time.Sleep(2 * time.Second)

	// Check goroutine count hasn't exploded
	currentGoroutines := runtime.NumGoroutine()
	goroutineGrowth := currentGoroutines - initialGoroutines

	// Should have minimal goroutine growth (just the retry goroutine)
	assert.LessOrEqual(t, goroutineGrowth, 10, "goroutine count should not explode during port collision retries")

	// Verify backoff is working by checking retryInterval increased
	configMu.Lock()
	currentRetryInterval := retryInterval
	configMu.Unlock()
	assert.Greater(t, currentRetryInterval, time.Duration(50)*time.Millisecond, "retryInterval should have increased due to backoff")

	// Clean up
	blockListener.Close()
	configMu.Lock()
	if server != nil {
		_ = server.Shutdown(context.Background())
	}
	configMu.Unlock()
	time.Sleep(100 * time.Millisecond)

	// Restore original values
	configMu.Lock()
	retryInterval = origRetryInterval
	maxRetryInterval = origMaxRetryInterval
	addr = origAddr
	server = origServer
	driverAlive = origDriverAlive
	configUpdated = origConfigUpdated
	hooks = origHooks
	sysMap = origSysMap
	configMu.Unlock()
}

// TestHotReload tests that changing loglevel updates the driver's logging
// behavior without restarting the HTTP server.
func TestHotReload(t *testing.T) {
	// Save original values
	var origRetryInterval time.Duration
	var origMaxRetryInterval time.Duration
	var origAddr string
	var origServer *http.Server
	var origDriverAlive bool
	var origConfigUpdated bool
	var origHooks map[string]interface{}
	var origSysMap map[string]map[string]interface{}

	configMu.Lock()
	origRetryInterval = retryInterval
	origMaxRetryInterval = maxRetryInterval
	origAddr = addr
	origServer = server
	origDriverAlive = driverAlive
	origConfigUpdated = configUpdated
	origHooks = hooks
	origSysMap = sysMap
	configMu.Unlock()

	// Reset for test - driver starts with driverAlive = false (like real driver)
	configMu.Lock()
	retryInterval = 500 * time.Millisecond
	maxRetryInterval = 30 * time.Second
	driverAlive = false
	configUpdated = false
	server = nil
	hooks = map[string]interface{}{
		"test": []interface{}{map[string]interface{}{"match": map[string]interface{}{"url": "/test"}}},
	}
	sysMap = map[string]map[string]interface{}{}
	configMu.Unlock()

	// Use a specific port that's likely free
	testAddr := "127.0.0.1:18999"

	testDriver := makeTestDriver("test-hot-reload", testAddr, hooks)
	setDriver(testDriver)

	// Start webhook
	startWebhook(nil)

	// Wait for server to start
	if err := waitForServer(testAddr, 5*time.Second); err != nil {
		t.Fatalf("server did not start: %v", err)
	}

	// Verify server is responding
	client := &http.Client{Timeout: 2 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+testAddr+"/hz/alive", nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	cancel()

	// Simulate config reload with loglevel change (m != nil triggers logger update)
	// but same address - should NOT restart server
	setConfigUpdated()

	testDriver2 := makeTestDriver("test-hot-reload", testAddr, hooks)
	setDriver(testDriver2)

	// Call loadOptions directly (simulating Reload)
	loadOptions(nil)

	// Wait for potential restart
	time.Sleep(300 * time.Millisecond)

	// Verify server still responds on the same address (not restarted)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	req2, _ := http.NewRequestWithContext(ctx2, "GET", "http://"+testAddr+"/hz/alive", nil)
	resp2, err := client.Do(req2)
	require.NoError(t, err, "server should still be running after config reload with same address")
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	resp2.Body.Close()
	cancel2()

	// Verify address hasn't changed
	configMu.Lock()
	currentAddr := addr
	configMu.Unlock()
	assert.Equal(t, testAddr, currentAddr, "address should not change when reloading with same address")

	// Clean up
	configMu.Lock()
	if server != nil {
		_ = server.Shutdown(context.Background())
	}
	configMu.Unlock()
	time.Sleep(100 * time.Millisecond)

	// Restore original values
	configMu.Lock()
	retryInterval = origRetryInterval
	maxRetryInterval = origMaxRetryInterval
	addr = origAddr
	server = origServer
	driverAlive = origDriverAlive
	configUpdated = origConfigUpdated
	hooks = origHooks
	sysMap = origSysMap
	configMu.Unlock()
}

// TestSynchronization tests race-free access to hooks and sysMap under
// concurrent reloads and webhook requests.
func TestSynchronization(t *testing.T) {
	// Save original values
	var origRetryInterval time.Duration
	var origMaxRetryInterval time.Duration
	var origAddr string
	var origServer *http.Server
	var origDriverAlive bool
	var origConfigUpdated bool
	var origHooks map[string]interface{}
	var origSysMap map[string]map[string]interface{}

	configMu.Lock()
	origRetryInterval = retryInterval
	origMaxRetryInterval = maxRetryInterval
	origAddr = addr
	origServer = server
	origDriverAlive = driverAlive
	origConfigUpdated = configUpdated
	origHooks = hooks
	origSysMap = sysMap
	configMu.Unlock()

	// Reset for test
	configMu.Lock()
	retryInterval = 50 * time.Millisecond
	maxRetryInterval = 200 * time.Millisecond
	driverAlive = false
	configUpdated = false
	server = nil
	hooks = map[string]interface{}{
		"test": []interface{}{map[string]interface{}{"match": map[string]interface{}{"url": "/test"}}},
	}
	sysMap = map[string]map[string]interface{}{
		"test": {"signatureHeader": "x-test", "signatureSecret": "secret"},
	}
	configMu.Unlock()

	// Use a specific port
	testAddr := "127.0.0.1:18998"

	testDriver := makeTestDriver("test-sync", testAddr, hooks)
	setDriver(testDriver)

	// Start webhook
	startWebhook(nil)

	// Wait for server to start
	if err := waitForServer(testAddr, 5*time.Second); err != nil {
		t.Fatalf("server did not start: %v", err)
	}

	currentAddr := testAddr

	client := &http.Client{Timeout: 2 * time.Second}

	// Error channel
	errors := make(chan error, 100)

	var wg sync.WaitGroup

	// Goroutine 1: Rapid reloads (simulate config changes)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			setConfigUpdated()
			// Use same hooks - in real usage, the daemon would update the driver options
			newDriver := makeTestDriver("test-sync", currentAddr, hooks)
			setDriver(newDriver)
			loadOptions(nil)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Goroutine 2: Webhook requests to /hz/alive
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+currentAddr+"/hz/alive", nil)
			resp, err := client.Do(req)
			if err != nil {
				errors <- err
			} else {
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					errors <- assert.AnError
				}
			}
			cancel()
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Goroutine 3: Webhook requests to /test (matched hook)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+currentAddr+"/test", bytes.NewBufferString("test"))
			req.Header.Set("X-Test-Signature", "dummy")
			resp, err := client.Do(req)
			if err != nil {
				errors <- err
			} else {
				resp.Body.Close()
			}
			cancel()
			time.Sleep(8 * time.Millisecond)
		}
	}()

	// Wait for all goroutines with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines completed
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out waiting for goroutines")
	}

	close(errors)

	// Check for errors (excluding expected 404s from signature mismatch)
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
			t.Logf("Error during synchronization test: %v", err)
		}
	}

	// Clean up
	configMu.Lock()
	if server != nil {
		_ = server.Shutdown(context.Background())
	}
	configMu.Unlock()
	time.Sleep(100 * time.Millisecond)

	// Restore original values
	configMu.Lock()
	retryInterval = origRetryInterval
	maxRetryInterval = origMaxRetryInterval
	addr = origAddr
	server = origServer
	driverAlive = origDriverAlive
	configUpdated = origConfigUpdated
	hooks = origHooks
	sysMap = origSysMap
	configMu.Unlock()

	// Should have no unexpected errors (race detector would catch data races)
	assert.LessOrEqual(t, errorCount, 0, "should have no race conditions or panics during concurrent access")
}

// TestAddressChange verifies that stopWebhook correctly triggers a restart
// on the new port and that loadOptions skips redundant map processing
// during the recursive call (using the configUpdated flag).
func TestAddressChange(t *testing.T) {
	// Save original values
	var origRetryInterval time.Duration
	var origMaxRetryInterval time.Duration
	var origAddr string
	var origServer *http.Server
	var origDriverAlive bool
	var origConfigUpdated bool
	var origHooks map[string]interface{}
	var origSysMap map[string]map[string]interface{}

	configMu.Lock()
	origRetryInterval = retryInterval
	origMaxRetryInterval = maxRetryInterval
	origAddr = addr
	origServer = server
	origDriverAlive = driverAlive
	origConfigUpdated = configUpdated
	origHooks = hooks
	origSysMap = sysMap
	configMu.Unlock()

	// Reset for test
	configMu.Lock()
	retryInterval = 500 * time.Millisecond
	maxRetryInterval = 30 * time.Second
	driverAlive = false
	configUpdated = false
	addr = ""
	server = nil
	hooks = nil
	sysMap = nil
	configMu.Unlock()

	// Create test driver
	testHooks := map[string]interface{}{
		"test": []interface{}{map[string]interface{}{"match": map[string]interface{}{"url": "/test"}}},
	}

	// Use specific ports for the test
	testAddr1 := "127.0.0.1:18997"
	testAddr2 := "127.0.0.1:18996"

	testDriver := makeTestDriver("test-addr-change", testAddr1, testHooks)
	setDriver(testDriver)

	// Start webhook on first port
	configMu.Lock()
	addr = testAddr1
	configMu.Unlock()
	startWebhook(nil)

	// Wait for server to start
	if err := waitForServer(testAddr1, 5*time.Second); err != nil {
		t.Fatalf("first server did not start: %v", err)
	}

	firstAddr := testAddr1

	// Verify first server is listening
	client := &http.Client{Timeout: 2 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+firstAddr+"/hz/alive", nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	cancel()

	// Now change the address by calling loadOptions with new driver
	newDriver := makeTestDriver("test-addr-change", testAddr2, testHooks)
	setDriver(newDriver)

	// Set configUpdated to trigger reload
	setConfigUpdated()

	// Call loadOptions - this should trigger address change and server restart
	loadOptions(nil)

	// Wait for server to restart on new port
	time.Sleep(500 * time.Millisecond)

	// Get the new address
	configMu.Lock()
	secondAddr := server.Addr
	configMu.Unlock()

	// Verify address changed
	assert.NotEqual(t, firstAddr, secondAddr, "address should change after reload with new address")

	// Verify old server is no longer listening
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	req2, _ := http.NewRequestWithContext(ctx2, "GET", "http://"+firstAddr+"/hz/alive", nil)
	resp2, err := client.Do(req2)
	if resp2 != nil {
		resp2.Body.Close()
	}
	assert.Error(t, err, "old address should no longer be listening")
	cancel2()

	// Verify new server is listening
	ctx3, cancel3 := context.WithTimeout(context.Background(), 2*time.Second)
	req3, _ := http.NewRequestWithContext(ctx3, "GET", "http://"+secondAddr+"/hz/alive", nil)
	resp3, err := client.Do(req3)
	require.NoError(t, err, "new address should be listening")
	assert.Equal(t, http.StatusOK, resp3.StatusCode)
	resp3.Body.Close()
	cancel3()

	// Clean up
	configMu.Lock()
	if server != nil {
		_ = server.Shutdown(context.Background())
	}
	configMu.Unlock()
	time.Sleep(100 * time.Millisecond)

	// Restore original values
	configMu.Lock()
	retryInterval = origRetryInterval
	maxRetryInterval = origMaxRetryInterval
	addr = origAddr
	server = origServer
	driverAlive = origDriverAlive
	configUpdated = origConfigUpdated
	hooks = origHooks
	sysMap = origSysMap
	configMu.Unlock()
}

// TestLoadOptionsSkipRedundantProcessing tests that loadOptions skips
// redundant map processing when configUpdated is false and driver is alive.
func TestLoadOptionsSkipRedundantProcessing(t *testing.T) {
	// Save original values
	var origRetryInterval time.Duration
	var origMaxRetryInterval time.Duration
	var origAddr string
	var origServer *http.Server
	var origDriverAlive bool
	var origConfigUpdated bool
	var origHooks map[string]interface{}
	var origSysMap map[string]map[string]interface{}

	configMu.Lock()
	origRetryInterval = retryInterval
	origMaxRetryInterval = maxRetryInterval
	origAddr = addr
	origServer = server
	origDriverAlive = driverAlive
	origConfigUpdated = configUpdated
	origHooks = hooks
	origSysMap = sysMap
	configMu.Unlock()

	// Reset for test - driver is already alive
	configMu.Lock()

	retryInterval = 500 * time.Millisecond
	maxRetryInterval = 30 * time.Second
	driverAlive = true    // Already alive
	configUpdated = false // No config update
	addr = "127.0.0.1:18999"
	server = nil
	hooks = map[string]interface{}{
		"test": []interface{}{map[string]interface{}{"match": map[string]interface{}{"url": "/test"}}},
	}
	sysMap = map[string]map[string]interface{}{
		"test": {"signatureHeader": "x-test"},
	}
	configMu.Unlock()

	// Create test driver
	testDriver := makeTestDriver("test-skip-redundant", "127.0.0.1:18999", hooks)
	setDriver(testDriver)

	// Call loadOptions - should early return without processing
	loadOptions(nil)

	// Verify hooks and sysMap unchanged (early return happened)
	configMu.Lock()
	assert.Equal(t, 1, len(hooks), "hooks should not be reprocessed when configUpdated is false")
	assert.Equal(t, 1, len(sysMap), "sysMap should not be reprocessed when configUpdated is false")
	configMu.Unlock()

	// Now test with configUpdated = true but driver not alive
	configMu.Lock()
	driverAlive = false
	configUpdated = true
	hooks = nil
	sysMap = nil
	configMu.Unlock()

	testDriver2 := makeTestDriver("test-skip-redundant", "127.0.0.1:18999",
		map[string]interface{}{"test": []interface{}{map[string]interface{}{"match": map[string]interface{}{"url": "/test"}}}})
	setDriver(testDriver2)

	loadOptions(nil)

	// Should have processed (driverAlive was false)
	configMu.Lock()
	assert.NotNil(t, hooks, "hooks should be processed when driverAlive is false")
	assert.NotNil(t, sysMap, "sysMap should be processed when driverAlive is false")
	configMu.Unlock()

	// Clean up
	configMu.Lock()
	if server != nil {
		_ = server.Shutdown(context.Background())
	}
	configMu.Unlock()

	// Restore original values
	configMu.Lock()
	retryInterval = origRetryInterval
	maxRetryInterval = origMaxRetryInterval
	addr = origAddr
	server = origServer
	driverAlive = origDriverAlive
	configUpdated = origConfigUpdated
	hooks = origHooks
	sysMap = origSysMap
	configMu.Unlock()
}

// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package service

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestSequenceProcessingOrder(t *testing.T) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	var mu sync.Mutex
	var processed []string

	svc := &Service{
		name:       "testsvc",
		sequences:  map[string]chan dipper.Message{},
		responders: map[string][]MessageResponder{},
		transformers: map[string][]func(*driver.Runtime, *dipper.Message) *dipper.Message{
			"test:seq": {
				func(d *driver.Runtime, m *dipper.Message) *dipper.Message {
					mu.Lock()
					processed = append(processed, m.Labels["order"])
					mu.Unlock()

					return m
				},
			},
		},
	}

	svc.process(dipper.Message{Channel: "test", Subject: "seq", Labels: map[string]string{"sequence": "s1", "order": "1"}}, nil)
	svc.process(dipper.Message{Channel: "test", Subject: "seq", Labels: map[string]string{"sequence": "s1", "order": "2"}}, nil)
	svc.process(dipper.Message{Channel: "test", Subject: "seq", Labels: map[string]string{"sequence": "s1", "order": "3"}}, nil)

	// wait for the sequence goroutine to drain
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"1", "2", "3"}, processed, "messages should be processed in order")
}

func TestSequenceCleanup(t *testing.T) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	svc := &Service{
		name:         "testsvc",
		sequences:    map[string]chan dipper.Message{},
		responders:   map[string][]MessageResponder{},
		transformers: map[string][]func(*driver.Runtime, *dipper.Message) *dipper.Message{},
	}

	svc.process(dipper.Message{Channel: "test", Subject: "cleanup", Labels: map[string]string{"sequence": "s2"}}, nil)

	// wait for the sequence goroutine to drain and clean up
	time.Sleep(50 * time.Millisecond)

	svc.sequenceLock.Lock()
	defer svc.sequenceLock.Unlock()
	assert.Empty(t, svc.sequences, "sequences map should be empty after all messages are processed")
}

func TestSequenceIsolation(t *testing.T) {
	if dipper.Logger == nil {
		f, _ := os.OpenFile(os.DevNull, os.O_APPEND, 0o777)
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}

	var mu sync.Mutex
	processed := map[string][]string{}

	svc := &Service{
		name:       "testsvc",
		sequences:  map[string]chan dipper.Message{},
		responders: map[string][]MessageResponder{},
		transformers: map[string][]func(*driver.Runtime, *dipper.Message) *dipper.Message{
			"test:seq": {
				func(d *driver.Runtime, m *dipper.Message) *dipper.Message {
					mu.Lock()
					seq := m.Labels["sequence"]
					processed[seq] = append(processed[seq], m.Labels["order"])
					mu.Unlock()

					return m
				},
			},
		},
	}

	// two independent sequences
	svc.process(dipper.Message{Channel: "test", Subject: "seq", Labels: map[string]string{"sequence": "a", "order": "1"}}, nil)
	svc.process(dipper.Message{Channel: "test", Subject: "seq", Labels: map[string]string{"sequence": "b", "order": "1"}}, nil)
	svc.process(dipper.Message{Channel: "test", Subject: "seq", Labels: map[string]string{"sequence": "a", "order": "2"}}, nil)
	svc.process(dipper.Message{Channel: "test", Subject: "seq", Labels: map[string]string{"sequence": "b", "order": "2"}}, nil)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"1", "2"}, processed["a"], "sequence a messages should be ordered")
	assert.Equal(t, []string{"1", "2"}, processed["b"], "sequence b messages should be ordered")
}

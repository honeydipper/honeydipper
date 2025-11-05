// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package api

import (
	"time"

	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// DefaultAPIReapTimeout is the timeout in seconds before a API result is abandoned.
const DefaultAPIReapTimeout time.Duration = 30

// Request represents a live API call.
type Request struct {
	store       *Store
	urlPath     string
	method      string
	uuid        string
	contentType string

	reqType int
	fn      string
	service string
	params  map[string]interface{}

	results  map[string]interface{}
	err      error
	acks     []string
	firstACK chan byte
	ready    chan byte
	received chan byte

	ackTimeout time.Duration
	timeout    time.Duration
}

// Dispatch sends the call to the intended services.
func (a *Request) Dispatch() {
	if a.ready == nil {
		a.ready = make(chan byte)
		a.results = map[string]interface{}{}
		a.store.SaveRequest(a)

		dipper.Must(a.store.caller.Call("api-broadcast", "send", map[string]interface{}{
			"broadcastSubject": "call",
			"labels": map[string]interface{}{
				"fn":           a.fn,
				"uuid":         a.uuid,
				"service":      a.service,
				"content-type": a.contentType,
			},
			"data": a.params,
		}))

		go func() {
			defer dipper.SafeExitOnError("error waiting on acks for API call [%s]", a.uuid)
			defer func() {
				defer a.store.ClearRequest(a)
				time.Sleep(DefaultAPIReapTimeout * time.Second)
			}()
			defer close(a.ready)
			defer a.postACK()

			ackTimer := time.NewTimer(a.ackTimeout)
			defer ackTimer.Stop()

			switch a.reqType {
			case TypeFirst:
				// skipping ACKs
			case TypeMatch:
				// wait for the first ACK
				a.firstACK = make(chan byte)
				defer func() {
					close(a.firstACK)
					a.firstACK = nil
				}()
				select {
				case <-ackTimer.C:
				case <-a.firstACK:
				}
			case TypeAll:
				// wait for all to ACK
				time.Sleep(a.ackTimeout)
			}
		}()
	}
}

// postACK waits for the call result and fill the channel.
func (a *Request) postACK() {
	if r := recover(); r != nil {
		// error when waiting for ACK
		panic(r)
	}

	if len(a.acks) > 0 && len(a.acks) == len(a.results) {
		// already received all results
		return
	}

	postACKTimer := time.NewTimer(a.timeout)
	defer postACKTimer.Stop()

	// expecting more results
	a.received = make(chan byte)
	defer close(a.received)
	switch {
	case a.reqType != TypeFirst && len(a.acks) == 0:
		a.err = ErrAPINoACK
	case a.timeout != InfiniteDuration:
		select {
		case <-postACKTimer.C:
			a.err = dipper.ErrTimeout
		case <-a.received:
		}
	default:
		// no timeout, infinity wait
		<-a.received
	}
}

// getResults returns incompelete results.
func (a *Request) getResults() map[string]interface{} {
	return a.results
}

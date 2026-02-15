// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package driver

import (
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

// NullDriverHandler provides support for null driver.
type NullDriverHandler struct {
	AcquireFunc     func()
	MetaFunc        func() *Meta
	PrepareFunc     func(input chan<- *dipper.Message)
	SendMessageFunc func(*dipper.Message)
	StartFunc       func(string)
	CloseFunc       func()
	WaitFunc        func()
}

// Acquire does nothing unless overridden with AcquireFunc.
func (h *NullDriverHandler) Acquire() {
	if h.AcquireFunc != nil {
		h.AcquireFunc()
	}
}

// Meta does nothing unless overridden with MetaFunc.
func (h *NullDriverHandler) Meta() *Meta {
	if h.MetaFunc != nil {
		return h.MetaFunc()
	}

	return nil
}

// Prepare does nothing unless overridden with PrepareFunc.
func (h *NullDriverHandler) Prepare(stream chan<- *dipper.Message) {
	if h.PrepareFunc != nil {
		h.PrepareFunc(stream)
	}
}

// Start does nothing unless overridden with StartFunc.
func (h *NullDriverHandler) Start(service string) {
	if h.StartFunc != nil {
		h.StartFunc(service)
	}
}

// SendMessage does nothing unless overridden with SendMessageFunc.
func (h *NullDriverHandler) SendMessage(msg *dipper.Message) {
	if h.SendMessageFunc != nil {
		h.SendMessageFunc(msg)
	}
}

// Close does nothing unless overridden with CloseFunc.
func (h *NullDriverHandler) Close() {
	if h.CloseFunc != nil {
		h.CloseFunc()
	}
}

// Wait does nothing unless overridden with WaitFunc.
func (h *NullDriverHandler) Wait() {
	if h.WaitFunc != nil {
		h.WaitFunc()
	}
}

// NewNullDriver creates a null driver handler.
func NewNullDriver(meta *Meta) *NullDriverHandler {
	return &NullDriverHandler{
		MetaFunc: func() *Meta {
			return meta
		},
	}
}

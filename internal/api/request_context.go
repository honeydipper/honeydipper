// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// RequestContext includes all functions store requires to work with a HttpRequest.
type RequestContext interface {
	AbortWithStatusJSON(int, interface{})
	IndentedJSON(int, interface{})
	ContentType() string
	Get(string) (interface{}, bool)
	Set(string, interface{})
	GetPath() string
	GetPayload(method string) map[string]interface{}
}

// GinRequestContext is a RequestContext implemented with gin.Context.
type GinRequestContext struct {
	gin *gin.Context
}

// AbortWithStatusJSON aborts the request with given code and content.
func (rc *GinRequestContext) AbortWithStatusJSON(code int, content interface{}) {
	rc.gin.AbortWithStatusJSON(code, content)
}

// IndentedJSON finishes the request with given code and content.
func (rc *GinRequestContext) IndentedJSON(code int, content interface{}) {
	rc.gin.IndentedJSON(code, content)
}

// ContentType returns the content type of the request.
func (rc *GinRequestContext) ContentType() string {
	return rc.gin.ContentType()
}

// Get gets a value associated with the key.
func (rc *GinRequestContext) Get(key string) (interface{}, bool) {
	return rc.gin.Get(key)
}

// Set stores a k/v pair.
func (rc *GinRequestContext) Set(key string, value interface{}) {
	rc.gin.Set(key, value)
}

// GetPath returns the full path of the request.
func (rc *GinRequestContext) GetPath() string {
	return rc.gin.Request.URL.Path
}

// GetPayload returns the query parameters from the request.
func (rc *GinRequestContext) GetPayload(method string) map[string]interface{} {
	payload := map[string]interface{}{}
	for _, p := range rc.gin.Params {
		payload[p.Key] = p.Value
	}

	if method == http.MethodPost || method == http.MethodPut {
		payload["body"] = string(dipper.Must(rc.gin.GetRawData()).([]byte))
	}

	for k, varr := range rc.gin.Request.Form {
		if len(varr) > 1 {
			payload[k] = varr
		} else {
			payload[k] = varr[0]
		}
	}

	return payload
}

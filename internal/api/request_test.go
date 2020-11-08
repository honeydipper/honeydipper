// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package api

import (
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/api/mock_api"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/honeydipper/honeydipper/pkg/dipper/mock_dipper"
	"github.com/imdario/mergo"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
)

type RequestTestCase struct {
	Subject         string
	ContentType     string `json:"content-type" mapstructure:"content-type"`
	Path            string
	Payload         map[string]interface{}
	Steps           []TestStep
	Returns         []ReturnMessage
	ExpectedCode    int
	ExpectedContent map[string]interface{}
	Config          interface{}
	Def             Def
	UUIDs           []string `json:"uuids" mapstructure:"uuids"`
	ShouldAuthorize bool
}

func requestTest(t *testing.T, caseName string) (*Store, *RequestTestCase) {
	var buffer, delta map[string]interface{}
	dipper.Must(yaml.Unmarshal(dipper.Must(ioutil.ReadFile("test_fixtures/common.yaml")).([]byte), &buffer))
	dipper.Must(yaml.Unmarshal(dipper.Must(ioutil.ReadFile(fmt.Sprintf("test_fixtures/%s.yaml", caseName))).([]byte), &delta))
	dipper.Must(mergo.Merge(&buffer, delta, mergo.WithOverride))

	c := &RequestTestCase{}
	dipper.Must(mapstructure.Decode(buffer, c))

	// convert all times from test definition to milliseconds
	c.Def.AckTimeout *= time.Millisecond
	c.Def.Timeout *= time.Millisecond
	for i := range c.Returns {
		c.Returns[i].Delay *= time.Millisecond
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockReqCtx.EXPECT().Get(gomock.Eq("subject")).Times(1).Return(c.Subject, c.Subject != "")
	if c.ShouldAuthorize {
		mockReqCtx.EXPECT().GetPath().Times(1).Return(c.Path)
		mockReqCtx.EXPECT().GetPayload(gomock.Eq(c.Def.Method)).Times(1).Return(c.Payload)
		mockReqCtx.EXPECT().ContentType().Times(1).Return(c.ContentType)
	}

	mockRPCCaller := mock_dipper.NewMockRPCCaller(ctrl)
	l := NewStore(mockRPCCaller)
	l.config = c.Config
	l.setupAuthorization()

	uuids := c.UUIDs
	nextUUID := func() string {
		uuid := uuids[0]
		uuids = uuids[1:]

		return uuid
	}
	l.newUUID = nextUUID

	if wt, ok := dipper.GetMapData(c.Config, "writeTimeout"); ok {
		l.writeTimeout = time.Millisecond * time.Duration(wt.(float64))
	} else {
		l.writeTimeout = time.Millisecond * 100
	}

	started := false
	for _, st := range c.Steps {
		mockRPCCaller.EXPECT().Call(gomock.Eq(st.Feature), gomock.Eq(st.Method), gomock.Eq(st.ExpectedMessage)).Times(1).DoAndReturn(func(_, _ string, _ map[string]interface{}) (interface{}, error) {
			if !started {
				started = true
				go func() {
					for _, st := range c.Returns {
						dipper.Logger.Warning("delaying %d ms", st.Delay)
						time.Sleep(st.Delay)
						switch st.Msg.Labels["type"] {
						case "ack":
							l.HandleAPIACK(st.Msg)
						case "result":
							l.HandleAPIReturn(st.Msg)
						}
					}
				}()
			}
			return st.ReturnMessage, st.Err
		})
	}

	if c.ExpectedCode >= 400 {
		mockReqCtx.EXPECT().AbortWithStatusJSON(gomock.Eq(c.ExpectedCode), gomock.Eq(c.ExpectedContent)).Times(1)
	} else {
		mockReqCtx.EXPECT().IndentedJSON(gomock.Eq(c.ExpectedCode), gomock.Eq(c.ExpectedContent)).Times(1)
	}

	l.HandleHTTPRequest(mockReqCtx, c.Def)

	return l, c
}

func TestTypeAllAPI(t *testing.T) {
	requestTest(t, "TypeAllAPI")
}

func TestTypeFirstAPI(t *testing.T) {
	requestTest(t, "TypeFirstAPI")
}

func TestTypeMatchAPI(t *testing.T) {
	requestTest(t, "TypeMatchAPI")
}

func TestTypeMatchAPINoMatch(t *testing.T) {
	requestTest(t, "TypeMatchAPINoMatch")
}

func TestTypeAllAPITimeout(t *testing.T) {
	requestTest(t, "TypeAllAPITimeout")
}

func TestTypeMatchAPILongRequest(t *testing.T) {
	l, c := requestTest(t, "TypeMatchAPILongRequest")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockReqCtx.EXPECT().GetPath().Times(1).Return(c.Def.Path)
	req := l.GetRequest(c.Def, mockReqCtx)
	assert.Equal(t, c.UUIDs[0], req.uuid)

	assert.NotPanics(t, func() { l.ClearRequest(req) })
}

func TestUnauthorizedAPI(t *testing.T) {
	requestTest(t, "UnauthorizedAPI")
}

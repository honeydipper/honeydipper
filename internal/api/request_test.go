// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package api

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/api/mock_api"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/honeydipper/honeydipper/pkg/dipper/mock_dipper"
	"github.com/stretchr/testify/assert"
)

type RequestTestCase struct {
	subject         map[string]interface{}
	contentType     string
	path            string
	payload         map[string]interface{}
	steps           []TestStep
	returns         []ReturnMessage
	expectedCode    int
	expectedContent map[string]interface{}
	config          interface{}
	def             Def
	uuids           []string
	shouldAuthorize bool
}

func requestTest(t *testing.T, c *RequestTestCase) *Store {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockReqCtx.EXPECT().Get(gomock.Eq("subject")).Times(1).Return(c.subject, c.subject != nil)
	if c.shouldAuthorize {
		mockReqCtx.EXPECT().GetPath().Times(1).Return(c.path)
		mockReqCtx.EXPECT().GetPayload(gomock.Eq(c.def.method)).Times(1).Return(c.payload)
		mockReqCtx.EXPECT().ContentType().Times(1).Return(c.contentType)
	}

	mockRPCCaller := mock_dipper.NewMockRPCCaller(ctrl)
	l := NewStore(mockRPCCaller)
	l.config = c.config

	uuids := c.uuids
	nextUUID := func() string {
		uuid := uuids[0]
		uuids = uuids[1:]
		return uuid
	}
	l.newUUID = nextUUID

	if wt, ok := dipper.GetMapData(c.config, "writeTimeout"); ok {
		l.writeTimeout = wt.(time.Duration)
	} else {
		l.writeTimeout = time.Millisecond * 100
	}

	for _, st := range c.steps {
		mockRPCCaller.EXPECT().Call(gomock.Eq(st.feature), gomock.Eq(st.method), gomock.Eq(st.expectedMessage)).Times(1).Return(st.returnMessage, st.err)
	}

	if c.expectedCode >= 400 {
		mockReqCtx.EXPECT().AbortWithStatusJSON(gomock.Eq(c.expectedCode), gomock.Eq(c.expectedContent)).Times(1)
	} else {
		mockReqCtx.EXPECT().IndentedJSON(gomock.Eq(c.expectedCode), gomock.Eq(c.expectedContent)).Times(1)
	}

	go func() {
		for _, st := range c.returns {
			time.Sleep(st.delay)
			switch st.msg.Labels["type"] {
			case "ack":
				l.HandleAPIACK(st.msg)
			case "result":
				l.HandleAPIReturn(st.msg)
			}
		}
	}()

	l.HandleHTTPRequest(mockReqCtx, c.def)
	return l
}

func TestTypeAllAPI(t *testing.T) {
	uuids := []string{"34ik-ijo3i4jt84932-aiau3kegkjrl"}
	c := &RequestTestCase{
		// cfg and api definition
		config: map[string]interface{}{
			"acls": map[string]interface{}{
				"test_type_all": []interface{}{
					map[string]interface{}{
						"subjects": []interface{}{
							map[string]interface{}{"role": "any"},
						},
						"type": "allow",
					},
				},
			},
		},
		def: Def{
			path:    "/test_type_all",
			name:    "test_type_all",
			method:  "GET",
			reqType: TypeAll,
			service: "foo",
		},
		shouldAuthorize: true,

		// input
		uuids:       uuids,
		subject:     map[string]interface{}{"role": "any"},
		contentType: "application/json",
		path:        "/test_type_all",
		payload:     map[string]interface{}{},

		steps: []TestStep{
			{
				feature: "api-broadcast",
				method:  "send",
				expectedMessage: map[string]interface{}{
					"broadcastSubject": "call",
					"labels": map[string]interface{}{
						"fn":           "test_type_all",
						"uuid":         uuids[0],
						"service":      "foo",
						"content-type": "application/json",
					},
					"data": map[string]interface{}{},
				},
				returnMessage: nil,
			},
		},

		// returned message from service
		returns: []ReturnMessage{
			{
				delay: time.Millisecond,
				msg: &dipper.Message{
					Labels: map[string]string{
						"type": "ack",
						"uuid": uuids[0],
						"from": "bar",
					},
				},
			},
			{
				delay: time.Millisecond,
				msg: &dipper.Message{
					Labels: map[string]string{
						"type": "result",
						"uuid": uuids[0],
						"from": "bar",
					},
					Payload: map[string]interface{}{
						"result": "all",
					},
				},
			},
		},

		// expected result
		expectedCode:    200,
		expectedContent: map[string]interface{}{"bar": map[string]interface{}{"result": "all"}},
	}

	requestTest(t, c)
}

func TestTypeFirstAPI(t *testing.T) {
	uuids := []string{"34ik-ijo3i4jt84932-aiau3kegkjrl"}
	c := &RequestTestCase{
		// cfg and api definition
		config: map[string]interface{}{
			"acls": map[string]interface{}{
				"test_type_first": []interface{}{
					map[string]interface{}{
						"subjects": []interface{}{
							map[string]interface{}{"role": "any"},
						},
						"type": "allow",
					},
				},
			},
		},
		def: Def{
			path:    "/test_type_first",
			name:    "test_type_first",
			method:  "GET",
			reqType: TypeFirst,
			service: "foo",
		},
		shouldAuthorize: true,

		// input
		uuids:       uuids,
		subject:     map[string]interface{}{"role": "any"},
		contentType: "application/json",
		path:        "/test_type_first",
		payload:     map[string]interface{}{},

		steps: []TestStep{
			{
				feature: "api-broadcast",
				method:  "send",
				expectedMessage: map[string]interface{}{
					"broadcastSubject": "call",
					"labels": map[string]interface{}{
						"fn":           "test_type_first",
						"uuid":         uuids[0],
						"service":      "foo",
						"content-type": "application/json",
					},
					"data": map[string]interface{}{},
				},
				returnMessage: nil,
			},
		},

		// returned message from service
		returns: []ReturnMessage{
			{
				delay: 2 * time.Millisecond,
				msg: &dipper.Message{
					Labels: map[string]string{
						"type": "result",
						"uuid": uuids[0],
						"from": "bar",
					},
					Payload: map[string]interface{}{
						"result": "all",
					},
				},
			},
		},

		// expected result
		expectedCode:    200,
		expectedContent: map[string]interface{}{"bar": map[string]interface{}{"result": "all"}},
	}

	requestTest(t, c)
}

func TestTypeMatchAPI(t *testing.T) {
	uuids := []string{"34ik-ijo3i4jt84932-aiau3kegkjrl"}
	c := &RequestTestCase{
		// cfg and api definition
		config: map[string]interface{}{
			"acls": map[string]interface{}{
				"test_type_match": []interface{}{
					map[string]interface{}{
						"subjects": []interface{}{
							map[string]interface{}{"role": "any"},
						},
						"type": "allow",
					},
				},
			},
		},
		def: Def{
			path:    "/test_type_match",
			name:    "test_type_match",
			method:  "GET",
			reqType: TypeMatch,
			service: "foo",
		},
		shouldAuthorize: true,

		// input
		uuids:       uuids,
		subject:     map[string]interface{}{"role": "any"},
		contentType: "application/json",
		path:        "/test_type_match",
		payload:     map[string]interface{}{},

		steps: []TestStep{
			{
				feature: "api-broadcast",
				method:  "send",
				expectedMessage: map[string]interface{}{
					"broadcastSubject": "call",
					"labels": map[string]interface{}{
						"fn":           "test_type_match",
						"uuid":         uuids[0],
						"service":      "foo",
						"content-type": "application/json",
					},
					"data": map[string]interface{}{},
				},
				returnMessage: nil,
			},
		},

		// returned message from service
		returns: []ReturnMessage{
			{
				delay: time.Millisecond,
				msg: &dipper.Message{
					Labels: map[string]string{
						"type": "ack",
						"uuid": uuids[0],
						"from": "bar",
					},
				},
			},
			{
				delay: time.Millisecond,
				msg: &dipper.Message{
					Labels: map[string]string{
						"type": "result",
						"uuid": uuids[0],
						"from": "bar",
					},
					Payload: map[string]interface{}{
						"result": "all",
					},
				},
			},
		},

		// expected result
		expectedCode:    200,
		expectedContent: map[string]interface{}{"bar": map[string]interface{}{"result": "all"}},
	}

	requestTest(t, c)
}

func TestTypeMatchAPINoMatch(t *testing.T) {
	uuids := []string{"34ik-ijo3i4jt84932-aiau3kegkjrl"}
	c := &RequestTestCase{
		// cfg and api definition
		config: map[string]interface{}{
			"acls": map[string]interface{}{
				"test_type_match": []interface{}{
					map[string]interface{}{
						"subjects": []interface{}{
							map[string]interface{}{"role": "any"},
						},
						"type": "allow",
					},
				},
			},
		},
		def: Def{
			path:    "/test_type_match",
			name:    "test_type_match",
			method:  "GET",
			reqType: TypeMatch,
			service: "foo",
		},
		shouldAuthorize: true,

		// input
		uuids:       uuids,
		subject:     map[string]interface{}{"role": "any"},
		contentType: "application/json",
		path:        "/test_type_match",
		payload:     map[string]interface{}{},

		steps: []TestStep{
			{
				feature: "api-broadcast",
				method:  "send",
				expectedMessage: map[string]interface{}{
					"broadcastSubject": "call",
					"labels": map[string]interface{}{
						"fn":           "test_type_match",
						"uuid":         uuids[0],
						"service":      "foo",
						"content-type": "application/json",
					},
					"data": map[string]interface{}{},
				},
				returnMessage: nil,
			},
		},

		// returned message from service
		returns: []ReturnMessage{},

		// expected result
		expectedCode:    404,
		expectedContent: map[string]interface{}{"error": "object not found"},
	}

	requestTest(t, c)
}

func TestTypeAllAPITimeout(t *testing.T) {
	uuids := []string{"34ik-ijo3i4jt84932-aiau3kegkjrl"}
	c := &RequestTestCase{
		// cfg and api definition
		config: map[string]interface{}{
			"acls": map[string]interface{}{
				"test_type_all": []interface{}{
					map[string]interface{}{
						"subjects": []interface{}{
							map[string]interface{}{"role": "any"},
						},
						"type": "allow",
					},
				},
			},
		},
		def: Def{
			path:    "/test_type_all",
			name:    "test_type_all",
			method:  "GET",
			reqType: TypeAll,
			service: "foo",
			timeout: time.Millisecond,
		},
		shouldAuthorize: true,

		// input
		uuids:       uuids,
		subject:     map[string]interface{}{"role": "any"},
		contentType: "application/json",
		path:        "/test_type_all",
		payload:     map[string]interface{}{},

		steps: []TestStep{
			{
				feature: "api-broadcast",
				method:  "send",
				expectedMessage: map[string]interface{}{
					"broadcastSubject": "call",
					"labels": map[string]interface{}{
						"fn":           "test_type_all",
						"uuid":         uuids[0],
						"service":      "foo",
						"content-type": "application/json",
					},
					"data": map[string]interface{}{},
				},
				returnMessage: nil,
			},
		},

		// returned message from service
		returns: []ReturnMessage{
			{
				delay: time.Millisecond,
				msg: &dipper.Message{
					Labels: map[string]string{
						"type": "ack",
						"uuid": uuids[0],
						"from": "bar",
					},
				},
			},
		},

		// expected result
		expectedCode:    500,
		expectedContent: map[string]interface{}{"error": "timeout"},
	}

	requestTest(t, c)
}

func TestTypeMatchAPILongRequest(t *testing.T) {
	uuids := []string{"34ik-ijo3i4jt84932-aiau3kegkjrl"}
	c := &RequestTestCase{
		// cfg and api definition
		config: map[string]interface{}{
			"acls": map[string]interface{}{
				"test_type_match": []interface{}{
					map[string]interface{}{
						"subjects": []interface{}{
							map[string]interface{}{"role": "any"},
						},
						"type": "allow",
					},
				},
			},
			"writeTimeout": time.Millisecond * 6,
		},
		def: Def{
			path:    "/test_type_match",
			name:    "test_type_match",
			method:  "GET",
			reqType: TypeMatch,
			service: "foo",
			timeout: InfiniteDuration,
		},
		shouldAuthorize: true,

		// input
		uuids:       uuids,
		subject:     map[string]interface{}{"role": "any"},
		contentType: "application/json",
		path:        "/test_type_match",
		payload:     map[string]interface{}{},

		steps: []TestStep{
			{
				feature: "api-broadcast",
				method:  "send",
				expectedMessage: map[string]interface{}{
					"broadcastSubject": "call",
					"labels": map[string]interface{}{
						"fn":           "test_type_match",
						"uuid":         uuids[0],
						"service":      "foo",
						"content-type": "application/json",
					},
					"data": map[string]interface{}{},
				},
				returnMessage: nil,
			},
		},

		// returned message from service
		returns: []ReturnMessage{
			{
				delay: time.Millisecond,
				msg: &dipper.Message{
					Labels: map[string]string{
						"type": "ack",
						"uuid": uuids[0],
						"from": "bar",
					},
				},
			},
			{
				delay: 20 * time.Millisecond,
				msg: &dipper.Message{
					Labels: map[string]string{
						"type": "result",
						"uuid": uuids[0],
						"from": "bar",
					},
					Payload: map[string]interface{}{
						"result": "all",
					},
				},
			},
		},

		// expected result
		expectedCode:    202,
		expectedContent: map[string]interface{}{"results": map[string]interface{}{}, "uuid": uuids[0]},
	}

	l := requestTest(t, c)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockReqCtx.EXPECT().GetPath().Times(1).Return(c.def.path)
	req := l.GetRequest(c.def, mockReqCtx)
	assert.Equal(t, uuids[0], req.uuid)

	assert.NotPanics(t, func() { l.ClearRequest(req) })
}

func TestUnauthorizedAPI(t *testing.T) {
	uuids := []string{"34ik-ijo3i4jt84932-aiau3kegkjrl"}
	c := &RequestTestCase{
		// cfg and api definition
		config: map[string]interface{}{
			"acls": map[string]interface{}{
				"test_type_match": []interface{}{
					map[string]interface{}{
						"subjects": "all",
						"type":     "deny",
					},
					map[string]interface{}{
						"subjects": []interface{}{
							map[string]interface{}{"role": "privileged"},
						},
						"type": "allow",
					},
				},
			},
		},
		def: Def{
			path:    "/test_type_match",
			name:    "test_type_match",
			method:  "GET",
			reqType: TypeFirst,
			service: "foo",
		},
		shouldAuthorize: false,

		// input
		uuids:       uuids,
		subject:     map[string]interface{}{"role": "someone"},
		contentType: "application/json",
		path:        "/test_type_match",
		payload:     map[string]interface{}{},

		steps: []TestStep{},

		// returned message from service
		returns: []ReturnMessage{},

		// expected result
		expectedCode:    403,
		expectedContent: map[string]interface{}{"errors": "not allowed"},
	}

	requestTest(t, c)
}

// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package api

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"dario.cat/mergo"
	"github.com/ghodss/yaml"
	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/v4/internal/api/mock_api"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper/mock_dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
)

type RequestTestCase struct {
	Subject         string
	Provider        string
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
	dipper.Must(yaml.Unmarshal(dipper.Must(os.ReadFile("test_fixtures/common.yaml")).([]byte), &buffer))
	dipper.Must(yaml.Unmarshal(dipper.Must(os.ReadFile(fmt.Sprintf("test_fixtures/%s.yaml", caseName))).([]byte), &delta))
	dipper.Must(mergo.Merge(&buffer, delta, mergo.WithOverride))

	c := &RequestTestCase{}
	dipper.Must(mapstructure.Decode(buffer, c))
	if c.Def.Local == nil {
		if d, ok := GetDefsByName()[c.Def.Name]; ok {
			c.Def.Local = d.Local
		}
	}

	// convert all times from test definition to milliseconds
	c.Def.AckTimeout *= time.Millisecond
	c.Def.Timeout *= time.Millisecond
	for i := range c.Returns {
		c.Returns[i].Delay *= time.Millisecond
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	principal := Principal{
		Subject:  c.Subject,
		Provider: c.Provider,
	}
	mockReqCtx.EXPECT().Get(gomock.Eq("principal")).AnyTimes().Return(principal, c.Subject != "" || c.Provider != "")
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
		mockRPCCaller.EXPECT().Call(gomock.Eq(st.Feature), gomock.Eq(st.Method), gomock.Eq(st.ExpectedMessage)).Times(1).DoAndReturn(func(_, _ string, _ map[string]interface{}, labels ...string) (interface{}, error) {
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

func TestTypeLocalUserProfileAPI(t *testing.T) {
	requestTest(t, "TypeLocalUserProfileAPI")
}

func TestTypeLocalSAMLMetadataAPI(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockReqCtx.EXPECT().GetPath().Times(1).Return("/api/auth/saml/metadata")
	mockReqCtx.EXPECT().GetPayload(gomock.Eq(http.MethodGet)).Times(1).Return(map[string]interface{}{})
	mockReqCtx.EXPECT().ContentType().Times(1).Return("application/json")
	mockReqCtx.EXPECT().Data(
		gomock.Eq(http.StatusOK),
		gomock.Eq("application/samlmetadata+xml; charset=utf-8"),
		gomock.Eq([]byte("<?xml version=\"1.0\"?><EntityDescriptor />")),
	).Times(1)

	mockRPCCaller := mock_dipper.NewMockRPCCaller(ctrl)
	mockRPCCaller.EXPECT().Call(
		gomock.Eq("driver:auth-saml"),
		gomock.Eq("saml_sp_metadata"),
		gomock.Eq(map[string]interface{}{}),
	).Times(1).Return([]byte(`{"metadata":"<?xml version=\"1.0\"?><EntityDescriptor />"}`), nil)

	store := NewStore(mockRPCCaller)
	store.writeTimeout = 100 * time.Millisecond

	def := Def{ReqType: TypeLocal, Method: http.MethodGet, Name: "samlSPMetadata", Local: samlSPMetadataHandler, AllowAnonymous: true}
	store.HandleHTTPRequest(mockReqCtx, def)
}

func TestRequestAttachPrincipalUserLabel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReqCtx := mock_api.NewMockRequestContext(ctrl)
	mockReqCtx.EXPECT().Get(gomock.Eq("principal")).AnyTimes().Return(Principal{
		Subject:     "charles-subject",
		ProfileName: "charles",
	}, true)
	mockReqCtx.EXPECT().GetPath().Times(1).Return("/api/events/123/interact")
	mockReqCtx.EXPECT().GetPayload(gomock.Eq(http.MethodPost)).Times(1).Return(map[string]interface{}{"sessionID": "123"})
	mockReqCtx.EXPECT().ContentType().Times(1).Return("application/json")
	mockReqCtx.EXPECT().AbortWithStatusJSON(gomock.Eq(http.StatusInternalServerError), gomock.Any()).Times(1)

	mockRPCCaller := mock_dipper.NewMockRPCCaller(ctrl)
	mockRPCCaller.EXPECT().Call(
		gomock.Eq("api-broadcast"),
		gomock.Eq("send"),
		gomock.AssignableToTypeOf(map[string]interface{}{}),
	).Times(1).DoAndReturn(func(_, _ string, params map[string]interface{}, _ ...string) ([]byte, error) {
		labels := params["labels"].(map[string]interface{})
		if labels["user"] != "charles" {
			t.Fatalf("expected forwarded principal user label, got %+v", labels)
		}

		return []byte("1"), nil
	})

	store := NewStore(mockRPCCaller)
	store.config = map[string]interface{}{}
	store.writeTimeout = time.Millisecond
	store.newUUID = func() string { return "req-1" }

	def := Def{Method: http.MethodPost, Name: "eventInteract", Service: "engine", ReqType: TypeFirst, AttachPrincipalUser: true, AllowAnonymous: true}
	store.HandleHTTPRequest(mockReqCtx, def)
}

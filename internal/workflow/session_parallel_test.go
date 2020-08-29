// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// +build !integration

package workflow

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

type DipperMsgMatcher struct {
	val         interface{}
	description string
}

func (e *DipperMsgMatcher) Matches(x interface{}) bool {
	// for thread operations, ignore the sessionID and resume_token

	m := x.(*dipper.Message)
	msg := *m
	msg.Labels = dipper.MustDeepCopy(m.Labels).(map[string]string)
	delete(msg.Labels, "sessionID")

	if c, ok := msg.Payload.(map[string]interface{})["ctx"]; ok {
		c = dipper.MustDeepCopy(c)
		delete(c.(map[string]interface{}), "resume_token")
		msg.Payload.(map[string]interface{})["ctx"] = c
	}

	return reflect.DeepEqual(x, e.val)
}

func (e *DipperMsgMatcher) String() string {
	return e.description
}

func DipperMsgEq(x interface{}) gomock.Matcher {
	return &DipperMsgMatcher{
		val:         x,
		description: fmt.Sprintf("%v", x),
	}
}

func TestWorkflowIterateParallel(t *testing.T) {
	syntheticTest(t, configStr, map[string]interface{}{
		"workflow": &config.Workflow{
			CallFunction: "foo_sys.bar_func",
			IterateParallel: []string{
				"item1",
				"item2",
				"item3",
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc": "",
						"_meta_name": "foo_sys.bar_func",
						"current":    "item1",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc": "",
						"_meta_name": "foo_sys.bar_func",
						"current":    "item2",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc": "",
						"_meta_name": "foo_sys.bar_func",
						"current":    "item3",
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "1",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "1",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "2",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "2",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "3",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "3",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	})
}

func TestWorkflowThreads(t *testing.T) {
	syntheticTest(t, configStr, map[string]interface{}{
		"workflow": &config.Workflow{
			Threads: []config.Workflow{
				{
					CallFunction: "foo_sys.bar_func",
					Local:        map[string]interface{}{"item": 1},
				},
				{
					CallFunction: "foo_sys.bar_func",
					Local:        map[string]interface{}{"item": 2},
				},
				{
					CallFunction: "foo_sys.bar_func",
					Local:        map[string]interface{}{"item": 3},
				},
			},
		},
		"msg": &dipper.Message{},
		"ctx": map[string]interface{}{},
		"asserts": func() {
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":    "",
						"_meta_name":    "foo_sys.bar_func",
						"item":          1,
						"thread_number": 0,
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":    "",
						"_meta_name":    "foo_sys.bar_func",
						"item":          2,
						"thread_number": 1,
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
			mockHelper.EXPECT().SendMessage(DipperMsgEq(&dipper.Message{
				Channel: "eventbus",
				Subject: "command",
				Labels:  map[string]string{}, Payload: map[string]interface{}{
					"ctx": map[string]interface{}{
						"_meta_desc":    "",
						"_meta_name":    "foo_sys.bar_func",
						"item":          3,
						"thread_number": 2,
					},
					"data":  map[string]interface{}{},
					"event": map[string]interface{}{},
					"function": config.Function{
						Target: config.Action{
							System:   "foo_sys",
							Function: "bar_func",
						},
					},
					"labels": emptyLabels,
				},
			})).Times(1)
		},
		"steps": []map[string]interface{}{
			{
				"sessionID": "1",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "1",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "2",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "2",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
			{
				"sessionID": "3",
				"msg": &dipper.Message{
					Channel: "eventbus",
					Subject: "return",
					Labels: map[string]string{
						"sessionID": "3",
						"status":    "success",
					},
				},
				"ctx": map[string]interface{}{},
			},
		},
	})
}

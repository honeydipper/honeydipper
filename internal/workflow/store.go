// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

// SessionStore stores session in memory and provides helper function for session to perform
type SessionStore struct {
	sessions          map[string]*Session
	suspendedSessions map[string]string
	GetConfig         func() *config.Config
	SendMessage       func(msg *dipper.Message)
}

// NewSessionStore initialize the session store
func NewSessionStore() *SessionStore {
	s := &SessionStore{
		sessions:          map[string]*Session{},
		suspendedSessions: map[string]string{},
	}
	dipper.InitIDMap(&s.sessions)
	return s
}

// Len returns the length of the sessions list
func (s *SessionStore) Len() int {
	return len(s.sessions)
}

// newSession creates the workflow session
func (s *SessionStore) newSession(parent string, wf *config.Workflow) *Session {
	var w = &Session{
		parent:   parent,
		store:    s,
		workflow: wf,
	}

	switch {
	case wf.Description != "":
		w.performing = wf.Description
	case wf.Function.Target.System != "":
		w.performing = wf.Function.Target.System + "." + wf.Function.Target.Function
	case wf.Function.Driver != "":
		w.performing = "driver:" + wf.Function.Driver + "." + wf.Function.RawAction
	case wf.CallFunction != "":
		w.performing = wf.CallFunction
	case wf.CallDriver != "":
		w.performing = wf.CallDriver
	default:
		w.performing = w.workflow.Name
	}
	dipper.Logger.Infof("[workflow] workflow [%s] instantiated with parent ID [%s]", w.performing, parent)

	if w.parent != "" {
		parentSession := dipper.IDMapGet(&s.sessions, w.parent).(*Session)
		w.event = parentSession.event
		w.ctx = dipper.MustDeepCopyMap(parentSession.ctx)

		delete(w.ctx, "hooks") // hooks don't get inherited
		w.loadedContexts = append([]string{}, parentSession.loadedContexts...)
	}

	return w
}

// StartSession starts a workflow session
func (s *SessionStore) StartSession(wf *config.Workflow, msg *dipper.Message, ctx map[string]interface{}) {
	defer dipper.SafeExitOnError("[workflow] error when creating workflow session")
	w := s.newSession("", wf)
	w.injectMsg(msg)
	w.initCTX()
	w.injectEventCTX(ctx)
	w.injectLocalCTX(msg)
	w.interpolateWorkflow(msg)
	w.injectMeta()

	w.execute(msg)
}

// ContinueSession resume a session with given dipper message
func (s *SessionStore) ContinueSession(sessionID string, msg *dipper.Message, export map[string]interface{}) {
	defer dipper.SafeExitOnError("[workflow] error when continuing workflow session %s", sessionID)
	w := dipper.IDMapGet(&s.sessions, sessionID).(*Session)
	if w != nil {
		defer w.onError()
		w.continueExec(msg, export)
		return
	}
	dipper.Logger.Warningf("waiting session is cleared or missing %s", sessionID)
}

// ResumeSession resume a session that is in waiting state
func (s *SessionStore) ResumeSession(key string, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[workflow] error when resuming session for key %s", key)
	sessionID, ok := s.suspendedSessions[key]
	if ok {
		delete(s.suspendedSessions, key)
		sessionPayload, _ := dipper.GetMapData(msg.Payload, "payload")
		sessionLabels := map[string]string{}
		if labels, ok := dipper.GetMapData(msg.Payload, "labels"); ok {
			err := mapstructure.Decode(labels, &sessionLabels)
			if err != nil {
				panic(err)
			}
		}
		go s.ContinueSession(sessionID, &dipper.Message{
			Subject: dipper.EventbusReturn,
			Labels:  sessionLabels,
			Payload: sessionPayload,
		}, nil)
	}
}

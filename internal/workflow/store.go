// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package workflow

import (
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/mitchellh/mapstructure"
)

// SessionStoreHelper enables SessionStore to actually drive the workflows and load configs.
type SessionStoreHelper interface {
	GetConfig() *config.Config
	SendMessage(msg *dipper.Message)
}

// SessionStore stores session in memory and provides helper function for session to perform.
type SessionStore struct {
	sessions          map[string]SessionHandler
	suspendedSessions map[string]string
	Helper            SessionStoreHelper
}

// NewSessionStore initialize the session store.
func NewSessionStore(helper SessionStoreHelper) *SessionStore {
	s := &SessionStore{
		sessions:          map[string]SessionHandler{},
		suspendedSessions: map[string]string{},
		Helper:            helper,
	}
	dipper.InitIDMap(&s.sessions)
	return s
}

// Len returns the length of the sessions list.
func (s *SessionStore) Len() int {
	return len(s.sessions)
}

// newSession creates the workflow session.
func (s *SessionStore) newSession(parent string, eventUUID string, wf *config.Workflow) SessionHandler {
	w := &Session{
		parent:   parent,
		store:    s,
		EventID:  eventUUID,
		workflow: wf,
	}

	performing := w.setPerforming("")
	dipper.Logger.Infof("[workflow] workflow [%s] instantiated with parent ID [%s]", performing, parent)

	return w
}

// StartSession starts a workflow session.
func (s *SessionStore) StartSession(wf *config.Workflow, msg *dipper.Message, ctx map[string]interface{}) {
	defer dipper.SafeExitOnError("[workflow] error when creating workflow session")
	eventUUID := msg.Labels["eventID"]
	w := s.newSession("", eventUUID, wf)
	w.prepare(msg, nil, ctx)
	w.execute(msg)
}

// ContinueSession resume a session with given dipper message.
func (s *SessionStore) ContinueSession(sessionID string, msg *dipper.Message, exports map[string]interface{}) {
	defer dipper.SafeExitOnError("[workflow] error when continuing workflow session %s", sessionID)
	w := dipper.IDMapGet(&s.sessions, sessionID).(SessionHandler)
	defer w.onError()
	w.continueExec(msg, exports)
}

// ResumeSession resume a session that is in waiting state.
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
				// if we panic here, the session wont be cleared from memory
				// so leave an empty labels map to cause later panic
				dipper.Logger.Warningf("[workflow] error when parsing resuming labels for %s: %+v", key, labels)
			}
		}
		daemon.Children.Add(1)
		go func() {
			defer daemon.Children.Done()
			s.ContinueSession(sessionID, &dipper.Message{
				Subject: dipper.EventbusReturn,
				Labels:  sessionLabels,
				Payload: sessionPayload,
			}, nil)
		}()
	}
}

// ByEventID retrieves all sessions that match the given EventID.
func (s *SessionStore) ByEventID(eventID string) []SessionHandler {
	var ret []SessionHandler
	for _, sh := range s.sessions {
		if sh.GetParent() == "" && sh.GetEventID() == eventID {
			ret = append(ret, sh)
		}
	}
	return ret
}

// GetEvents retrieves all sessions that are directly triggered through events.
func (s *SessionStore) GetEvents() []SessionHandler {
	var ret []SessionHandler
	for _, sh := range s.sessions {
		if sh.GetParent() == "" {
			ret = append(ret, sh)
		}
	}
	return ret
}

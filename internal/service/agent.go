// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"strconv"
	"sync/atomic"

	"github.com/honeydipper/honeydipper/v4/internal/config"
	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

var (
	agent             *Service
	agentMatchCounter int64
)

// StartAgent starts the agent service.
func StartAgent(cfg *config.Config) {
	agent = NewService(cfg, "agent")

	agent.EmitMetrics = agentMetrics
	agent.addResponder("eventbus:activate", createActivations)

	agent.start()
}

func createActivations(_ *driver.Runtime, msg *dipper.Message) {
	defer dipper.SafeExitOnError("[agent] continue processing activate rules")
	<-agent.Ready()
	msg = dipper.DeserializePayload(msg)

	agentName, ok := dipper.GetMapDataStr(msg.Payload, "agent")
	if !ok || agentName == "" {
		return
	}

	atomic.AddInt64(&agentMatchCounter, 1)
	dipper.Logger.Infof("[agent] activation request received for agent %s", agentName)
}

func agentMetrics() {
	agent.GaugeSet("honey.honeydipper.agent.activations", strconv.FormatInt(atomic.LoadInt64(&agentMatchCounter), 10), []string{})
}

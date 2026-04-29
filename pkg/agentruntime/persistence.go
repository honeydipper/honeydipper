// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package agentruntime

import (
	"encoding/json"
	"fmt"

	"github.com/honeydipper/honeydipper/v4/pkg/agenthistory"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

func SaveJSON(caller dipper.RPCCaller, key string, data interface{}, ttl string) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}
	_, err = caller.Call("cache", "save", map[string]any{
		"key":   key,
		"value": string(b),
		"ttl":   ttl,
	})
	if err != nil {
		return fmt.Errorf("save %s: %w", key, err)
	}

	return nil
}

func LoadJSON(caller dipper.RPCCaller, key string, out interface{}) (bool, error) {
	raw, err := caller.Call("cache", "load", map[string]any{"key": key})
	if err != nil {
		return false, fmt.Errorf("load %s: %w", key, err)
	}
	if len(raw) == 0 {
		return false, nil
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return false, fmt.Errorf("unmarshal %s: %w", key, err)
	}

	return true, nil
}

func LoadHistory(caller dipper.RPCCaller, key string) ([]agenthistory.TurnHistoryRecord, error) {
	raw, err := caller.Call("cache", "lrange", map[string]any{"key": key})
	if err != nil {
		return nil, fmt.Errorf("load history %s: %w", key, err)
	}
	history, err := agenthistory.ParseHistory(raw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal history %s: %w", key, err)
	}

	return history, nil
}

func SeedHistory(caller dipper.RPCCaller, key string, prompt string, ttl string) ([]agenthistory.TurnHistoryRecord, error) {
	record := agenthistory.TurnHistoryRecord{
		Role: agenthistory.RoleSystem,
		Content: &agenthistory.MessageContent{
			Text: prompt,
		},
	}
	b, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal seeded history %s: %w", key, err)
	}
	if _, err := caller.Call("cache", "rpush", map[string]any{
		"key":   key,
		"value": string(b),
		"ttl":   ttl,
	}); err != nil {
		return nil, fmt.Errorf("seed history %s: %w", key, err)
	}

	return []agenthistory.TurnHistoryRecord{record}, nil
}

// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration

package main

import (
	"testing"
	"time"

	"github.com/go-redis/redismock/v8"
	"github.com/honeydipper/honeydipper/v4/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLock_UsesTurnOwnerOrLegacyNode(t *testing.T) {
	dipper.GetLogger("redislock-test", "ERROR")
	for _, owner := range []string{"", "session-1"} {
		t.Run(owner, func(t *testing.T) {
			db, mock := redismock.NewClientMock()
			defer db.Close()
			locker := &Locker{
				driver: dipper.NewDriver("test", "redislock"), prefix: DefaultPrefix, nodeID: "node",
				redisOptions: &redisclient.Options{Client: db},
			}
			value := owner
			if value == "" {
				value = "node"
			}
			mock.ExpectSetNX("lock:turn", value, time.Minute).SetVal(true)
			msg := &dipper.Message{
				Payload: map[string]interface{}{"name": "turn", "expire": "1m", "owner": owner},
				Reply:   make(chan dipper.Message, 1),
			}
			locker.lock(msg)
			require.Len(t, msg.Reply, 1)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUnlock_OwnerCheckIsAtomicAndMissingOrReplacedLockIsSafe(t *testing.T) {
	dipper.GetLogger("redislock-test", "ERROR")
	for _, deleted := range []int64{0, 1} {
		db, mock := redismock.NewClientMock()
		defer db.Close()
		locker := &Locker{
			driver: dipper.NewDriver("test", "redislock"), prefix: DefaultPrefix,
			redisOptions: &redisclient.Options{Client: db},
		}
		// A single compare-and-delete operation must replace unconditional DEL.
		// Redis returns zero both when the key is gone and when a successor owns it.
		mock.ExpectEval(unlockOwnedScript, []string{"lock:turn"}, "session-1").SetVal(deleted)
		msg := &dipper.Message{
			Payload: map[string]interface{}{"name": "turn", "owner": "session-1"},
			Reply:   make(chan dipper.Message, 1),
		}
		assert.NotPanics(t, func() { locker.unlock(msg) })
		require.Len(t, msg.Reply, 1)
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestUnlock_LegacyMissingLockStillFails(t *testing.T) {
	dipper.GetLogger("redislock-test", "ERROR")
	db, mock := redismock.NewClientMock()
	defer db.Close()
	locker := &Locker{
		driver: dipper.NewDriver("test", "redislock"), prefix: DefaultPrefix,
		redisOptions: &redisclient.Options{Client: db},
	}
	mock.ExpectDel("lock:turn").SetVal(0)
	msg := &dipper.Message{
		Payload: map[string]interface{}{"name": "turn"}, Reply: make(chan dipper.Message, 1),
	}
	assert.PanicsWithValue(t, ErrFailToUnlock, func() { locker.unlock(msg) })
	require.NoError(t, mock.ExpectationsWereMet())
}

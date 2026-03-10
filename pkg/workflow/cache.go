// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package workflow implements workflow execution and state management.
package workflow

import (
	"encoding/json"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const LabelFromCache = "from_cache"

func (w *Session) processCheckCacheState() {
	delete(w.Ctx, "cache-data")
	delete(w.CurrentMsg.Labels, LabelFromCache)

	if force, ok := dipper.GetMapDataBool(w.Ctx, "force-cache-refresh"); ok && force {
		w.store.GetLogger().Warningf("[%s.%s] force cache refreshing %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)

		return
	}

	var data map[string]any

	memcache := w.store.GetCache()
	if memcache != nil {
		if item := memcache.Get(w.Workflow.CacheKey); item != nil {
			data = item.Value()
		}
	}

	if data == nil {
		buf, _ := w.store.Call("cache", "load", map[string]any{
			"key": "workflow-cache/" + w.Workflow.CacheKey,
		})
		if len(buf) == 0 {
			w.store.GetLogger().Warningf("[%s.%s] cache missing for %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)

			return
		}
		dipper.Must(json.Unmarshal(buf, &data))
	}

	w.Ctx["cache-data"] = data
	w.CurrentMsg.Labels[LabelFromCache] = "true"
	w.CurrentMsg.Labels["status"] = SessionStatusSuccess
	w.Exported = append(w.Exported, map[string]any{"cache-data*": data})
}

func (w *Session) processSaveCacheState() {
	if _, ok := w.CurrentMsg.Labels[LabelFromCache]; ok {
		return
	}
	ttl := w.Workflow.CacheTTL
	if ttl == "" {
		ttl = "24h"
	}
	if data, ok := w.Ctx["cache-data"]; ok && data != nil {
		if memcache := w.store.GetCache(); memcache != nil {
			t := dipper.Must(time.ParseDuration(ttl)).(time.Duration)
			if t > time.Hour {
				t = time.Hour
			}
			memcache.Set(w.Workflow.CacheKey, data.(map[string]any), t)
		}

		buf := dipper.Must(json.Marshal(data)).([]byte)
		dipper.Must(w.store.Call("cache", "save", map[string]any{
			"key":   "workflow-cache/" + w.Workflow.CacheKey,
			"value": string(buf),
			"ttl":   ttl,
		}))

		w.store.GetLogger().Infof("[%s.%s] cache saved for %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)
	}
}

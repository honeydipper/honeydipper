// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package workflow implements workflow execution and state management.
package workflow

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

const LabelFromCache = "from_cache"

// cacheKeyMode enumerates how a CacheKey should be interpreted when loading or
// saving workflow cache data.
type cacheKeyMode int

const (
	// cacheKeyModeNormal targets a plain key with no '#' (the legacy behavior).
	cacheKeyModeNormal cacheKeyMode = iota
	// cacheKeyModeHashField targets a single field of a Redis hash, written as
	// "name#field". Each field is stored and TTL'd as an independent entry.
	cacheKeyModeHashField
	// cacheKeyModeWholeHash targets an entire Redis hash, written as "name#".
	// Whole-hash read-only is implemented in a later phase; for now it falls
	// through to the legacy load/save behavior like a normal key.
	cacheKeyModeWholeHash
)

// parseCacheKey splits a CacheKey on the first '#' into the hash name and field
// components and reports which caching mode applies. When no '#' is present the
// key is treated as a normal (legacy) key.
func parseCacheKey(key string) (hashName, field string, mode cacheKeyMode) {
	parts := strings.SplitN(key, "#", 2)
	if len(parts) == 1 {
		return "", "", cacheKeyModeNormal
	}

	hashName, field = parts[0], parts[1]
	if field == "" {
		return hashName, "", cacheKeyModeWholeHash
	}

	return hashName, field, cacheKeyModeHashField
}

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
		hashName, field, mode := parseCacheKey(w.Workflow.CacheKey)
		switch mode {
		case cacheKeyModeHashField:
			buf, _ := w.store.Call("cache", "hget", map[string]any{
				"key":   "workflow-cache/" + hashName,
				"field": field,
			})
			if len(buf) == 0 {
				w.store.GetLogger().Warningf("[%s.%s] cache missing for %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)

				return
			}
			dipper.Must(json.Unmarshal(buf, &data))
		default:
			// Normal keys and whole-hash ("name#") keys use the existing load
			// behavior. Whole-hash read-only is implemented in a later phase.
			buf, _ := w.store.Call("cache", "load", map[string]any{
				"key": "workflow-cache/" + w.Workflow.CacheKey,
			})
			if len(buf) == 0 {
				w.store.GetLogger().Warningf("[%s.%s] cache missing for %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)

				return
			}
			dipper.Must(json.Unmarshal(buf, &data))
		}
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

		hashName, field, mode := parseCacheKey(w.Workflow.CacheKey)
		switch mode {
		case cacheKeyModeHashField:
			// Store a single hash field as an independent, TTL'd entry. The
			// redis-cache driver applies per-field HEXPIRE (when enabled) or
			// whole-hash EXPIRE to the underlying hash key.
			buf := dipper.Must(json.Marshal(data)).([]byte)
			dipper.Must(w.store.Call("cache", "hset", map[string]any{
				"key":   "workflow-cache/" + hashName,
				"value": map[string]any{field: string(buf)},
				"ttl":   ttl,
			}))
		default:
			// Normal keys and whole-hash ("name#") keys use the existing save
			// behavior. Whole-hash read-only is implemented in a later phase.
			buf := dipper.Must(json.Marshal(data)).([]byte)
			dipper.Must(w.store.Call("cache", "save", map[string]any{
				"key":   "workflow-cache/" + w.Workflow.CacheKey,
				"value": string(buf),
				"ttl":   ttl,
			}))
		}

		w.store.GetLogger().Infof("[%s.%s] cache saved for %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)
	}
}

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
	// The whole hash is read directly from Redis (hgetall) as a read-only
	// aggregate view; it is never written back.
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

// decodeCacheField returns the decoded value for a single Redis hash field. Hash
// fields store JSON-encoded values, so a string field is unmarshaled; any other
// type is returned as-is.
func decodeCacheField(value any) any {
	if s, ok := value.(string); ok {
		var decoded any
		dipper.Must(json.Unmarshal([]byte(s), &decoded))

		return decoded
	}

	return value
}

// loadWholeHashCache reads the entire Redis hash for hashName via the cache
// feature's hgetall RPC and aggregates it into a cache-data map keyed by field.
// It is a read-only operation: the caller must never write the result back. The
// second return value reports whether usable data was found.
func (w *Session) loadWholeHashCache(hashName string) (map[string]any, bool) {
	if hashName == "" {
		w.store.GetLogger().Warningf("[%s.%s] cache missing for %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)

		return nil, false
	}

	buf, _ := w.store.Call("cache", "hgetall", map[string]any{
		"key": "workflow-cache/" + hashName,
	})
	if len(buf) == 0 {
		w.store.GetLogger().Warningf("[%s.%s] cache missing for %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)

		return nil, false
	}

	raw := map[string]any{}
	dipper.Must(json.Unmarshal(buf, &raw))
	if len(raw) == 0 {
		w.store.GetLogger().Warningf("[%s.%s] cache missing for %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)

		return nil, false
	}

	data := make(map[string]any, len(raw))
	for field, value := range raw {
		data[field] = decodeCacheField(value)
	}

	return data, true
}

func (w *Session) processCheckCacheState() {
	delete(w.Ctx, "cache-data")
	delete(w.CurrentMsg.Labels, LabelFromCache)

	if force, ok := dipper.GetMapDataBool(w.Ctx, "force-cache-refresh"); ok && force {
		w.store.GetLogger().Warningf("[%s.%s] force cache refreshing %s", w.ID, w.CurrentMsg.Labels["cursor"], w.Workflow.CacheKey)

		return
	}

	hashName, field, mode := parseCacheKey(w.Workflow.CacheKey)

	var data map[string]any

	// Whole-hash ("name#") caching reads directly from Redis via hgetall and
	// never touches the memcache; skip the memcache lookup for that mode.
	if mode != cacheKeyModeWholeHash {
		memcache := w.store.GetCache()
		if memcache != nil {
			if item := memcache.Get(w.Workflow.CacheKey); item != nil {
				data = item.Value()
			}
		}
	}

	if data == nil {
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
		case cacheKeyModeWholeHash:
			// Read-only aggregate view of the entire Redis hash. We never write
			// back (see processSaveCacheState) and we ignore the memcache.
			whole, ok := w.loadWholeHashCache(hashName)
			if !ok {
				return
			}
			data = whole
		default:
			// Normal keys use the existing load behavior.
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

	// Whole-hash ("name#") caching is strictly read-only: never write back to
	// the memcache or to Redis, regardless of how the session was loaded (this
	// also covers the force-refresh / empty-hash run path).
	if _, _, mode := parseCacheKey(w.Workflow.CacheKey); mode == cacheKeyModeWholeHash {
		return
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
			// Normal keys use the existing save behavior.
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

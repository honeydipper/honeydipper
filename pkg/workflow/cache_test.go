// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

//go:build !integration
// +build !integration

package workflow

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/jellydator/ttlcache/v3"
)

type cacheTestStore struct {
	execTestStore
	memcache        *ttlcache.Cache[string, map[string]any]
	memcacheEnabled bool
	callReturn      func(string, string) ([]byte, error)
}

func (s *cacheTestStore) Call(feature, method string, params interface{}, labelsKV ...string) ([]byte, error) {
	s.lastCallFeature = feature
	s.lastCallMethod = method
	s.lastCallParams = params
	if s.callReturn != nil {
		return s.callReturn(feature, method)
	}

	return nil, nil
}
func (s *cacheTestStore) Stop() {}
func (s *cacheTestStore) GetCache() *ttlcache.Cache[string, map[string]any] {
	if !s.memcacheEnabled {
		return nil
	}
	if s.memcache == nil {
		s.memcache = ttlcache.New[string, map[string]any](ttlcache.WithTTL[string, map[string]any](time.Hour))
	}

	return s.memcache
}

func TestProcessCheckCacheState_WithCachedData(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{}
	es.callReturn = func(feature, method string) ([]byte, error) {
		if feature == "cache" && method == "load" {
			return []byte("{\"data\": \"cached-result\"}"), nil
		}

		return nil, nil
	}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}

	s.processCheckCacheState()

	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("expected from_cache label to be set")
	}
	if s.CurrentMsg.Labels["status"] != SessionStatusSuccess {
		t.Error("expected status to be success")
	}
	if s.Ctx["cache-data"] == nil {
		t.Error("expected cache-data in context")
	}
	if len(s.Exported) == 0 {
		t.Error("expected exported data")
	}
}

func TestProcessCheckCacheState_WithoutCachedData(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{LabelFromCache: "true"}}

	s.processCheckCacheState()

	if _, ok := s.CurrentMsg.Labels[LabelFromCache]; ok {
		t.Error("expected from-cache label to be removed")
	}
	if s.CurrentMsg.Labels["status"] == SessionStatusSuccess {
		t.Error("status should not be set when cache miss")
	}
}

func TestProcessSaveCacheState_SavesData(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.Workflow.CacheTTL = ""
	s.Ctx["cache-data"] = map[string]any{"result": "data"}

	s.processSaveCacheState()

	if es.lastCallFeature != "cache" || es.lastCallMethod != "save" {
		t.Errorf("expected cache save call, got %s.%s", es.lastCallFeature, es.lastCallMethod)
	}
	params, ok := es.lastCallParams.(map[string]any)
	if !ok {
		t.Fatal("params should be map")
	}
	if params["key"] != "workflow-cache/test-key" {
		t.Errorf("unexpected cache key: %v", params["key"])
	}
}

func TestProcessSaveCacheState_SkipsWhenFromCache(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.Ctx["cache-data"] = map[string]any{"result": "data"}
	s.CurrentMsg.Labels[LabelFromCache] = "true"

	s.processSaveCacheState()

	if es.lastCallFeature == "cache" {
		t.Error("should not save cache when from-cache is set")
	}
}

func TestProcessCheckCacheState_ForceCacheRefresh(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{}
	es.callReturn = func(feature, method string) ([]byte, error) {
		if feature == "cache" && method == "load" {
			return []byte("{\"data\": \"cached-result\"}"), nil
		}

		return nil, nil
	}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}
	s.Ctx["force-cache-refresh"] = true

	s.processCheckCacheState()

	if s.CurrentMsg.Labels[LabelFromCache] == "true" {
		t.Error("from_cache label should not be set when force refresh")
	}
	if s.CurrentMsg.Labels["status"] == SessionStatusSuccess {
		t.Error("status should not be set when force refresh")
	}
	if s.Ctx["cache-data"] != nil {
		t.Error("cache-data should not be set when force refresh")
	}
}

func TestProcessCheckCacheState_ForceCacheRefreshFalse(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{}
	es.callReturn = func(feature, method string) ([]byte, error) {
		if feature == "cache" && method == "load" {
			return []byte("{\"data\": \"cached-result\"}"), nil
		}

		return nil, nil
	}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}
	s.Ctx["force-cache-refresh"] = false

	s.processCheckCacheState()

	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("from_cache label should be set when force refresh is false")
	}
	if s.CurrentMsg.Labels["status"] != SessionStatusSuccess {
		t.Error("status should be success when cache hit")
	}
}

func TestProcessCheckCacheState_MemcacheDisabled(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: false}
	es.callReturn = func(feature, method string) ([]byte, error) {
		if feature == "cache" && method == "load" {
			return []byte("{\"data\": \"external-cache\"}"), nil
		}

		return nil, nil
	}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}

	s.processCheckCacheState()

	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("expected from_cache label to be set")
	}
	if es.lastCallFeature != "cache" {
		t.Error("should call external cache when memcache disabled")
	}
	data := s.Ctx["cache-data"].(map[string]any)
	if data["data"] != "external-cache" {
		t.Errorf("expected external cache data, got %v", data)
	}
}

func TestProcessSaveCacheState_MemcacheDisabled(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: false}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.Workflow.CacheTTL = "1h"
	s.Ctx["cache-data"] = map[string]any{"result": "data"}

	s.processSaveCacheState()

	if es.lastCallFeature != "cache" || es.lastCallMethod != "save" {
		t.Errorf("expected cache save call, got %s.%s", es.lastCallFeature, es.lastCallMethod)
	}
	if es.GetCache() != nil {
		t.Error("memcache should be nil when disabled")
	}
}

func TestProcessCheckCacheState_MemcacheEnabled_Hit(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: true}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}

	es.GetCache().Set("test-key", map[string]any{"data": "memcached-result"}, time.Hour)

	s.processCheckCacheState()

	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("expected from_cache label to be set")
	}
	if s.Ctx["cache-data"] == nil {
		t.Error("expected cache-data in context")
	}
	data := s.Ctx["cache-data"].(map[string]any)
	if data["data"] != "memcached-result" {
		t.Errorf("expected memcached data, got %v", data)
	}
	if es.lastCallFeature == "cache" {
		t.Error("should not call external cache when memcache hit")
	}
}

func TestProcessCheckCacheState_MemcacheEnabled_MissFallback(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: true}
	es.callReturn = func(feature, method string) ([]byte, error) {
		if feature == "cache" && method == "load" {
			return []byte("{\"data\": \"external-cache\"}"), nil
		}

		return nil, nil
	}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}

	s.processCheckCacheState()

	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("expected from_cache label to be set")
	}
	data := s.Ctx["cache-data"].(map[string]any)
	if data["data"] != "external-cache" {
		t.Errorf("expected external cache data, got %v", data)
	}
	if es.lastCallFeature != "cache" {
		t.Error("should call external cache on memcache miss")
	}
}

func TestProcessSaveCacheState_MemcacheEnabled(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: true}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.Workflow.CacheTTL = "30m"
	s.Ctx["cache-data"] = map[string]any{"result": "data"}

	s.processSaveCacheState()

	item := es.GetCache().Get("test-key")
	if item == nil {
		t.Fatal("expected data in memcache")
	}
	data := item.Value()
	if data["result"] != "data" {
		t.Errorf("unexpected memcache data: %v", data)
	}
	if es.lastCallFeature != "cache" || es.lastCallMethod != "save" {
		t.Error("should also save to external cache")
	}
}

func TestProcessSaveCacheState_MemcacheEnabled_TTLCapped(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: true}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.Workflow.CacheTTL = "48h"
	s.Ctx["cache-data"] = map[string]any{"result": "data"}

	s.processSaveCacheState()

	item := es.GetCache().Get("test-key")
	if item == nil {
		t.Fatal("expected data in memcache")
	}
}

func TestProcessCheckCacheState_HashField_MemcacheHit(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: true}
	s.store = es
	s.Workflow.CacheKey = "myhash#myfield"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}
	es.GetCache().Set("myhash#myfield", map[string]any{"data": "memcached-result"}, time.Hour)

	s.processCheckCacheState()

	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("expected from_cache label to be set")
	}
	data := s.Ctx["cache-data"].(map[string]any)
	if data["data"] != "memcached-result" {
		t.Errorf("expected memcached data, got %v", data)
	}
	if es.lastCallFeature == "cache" {
		t.Error("should not call external cache when memcache hit")
	}
}

func TestProcessCheckCacheState_HashField_MissHget(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: false}
	es.callReturn = func(feature, method string) ([]byte, error) {
		if feature == "cache" && method == "hget" {
			return []byte(`{"data": "hash-field-result"}`), nil
		}

		return nil, nil
	}
	s.store = es
	s.Workflow.CacheKey = "myhash#myfield"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}

	s.processCheckCacheState()

	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("expected from_cache label to be set")
	}
	if es.lastCallFeature != "cache" || es.lastCallMethod != "hget" {
		t.Errorf("expected cache hget call, got %s.%s", es.lastCallFeature, es.lastCallMethod)
	}
	params, ok := es.lastCallParams.(map[string]any)
	if !ok {
		t.Fatalf("params should be map")
	}
	if params["key"] != "workflow-cache/myhash" {
		t.Errorf("unexpected hget key: %v", params["key"])
	}
	if params["field"] != "myfield" {
		t.Errorf("unexpected hget field: %v", params["field"])
	}
	data := s.Ctx["cache-data"].(map[string]any)
	if data["data"] != "hash-field-result" {
		t.Errorf("expected hash field data, got %v", data)
	}
}

func TestProcessSaveCacheState_HashField(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: true}
	s.store = es
	s.Workflow.CacheKey = "myhash#myfield"
	s.Workflow.CacheTTL = "30m"
	s.Ctx["cache-data"] = map[string]any{"result": "data"}

	s.processSaveCacheState()

	item := es.GetCache().Get("myhash#myfield")
	if item == nil {
		t.Fatal("expected data in memcache for hash field key")
	}
	if es.lastCallFeature != "cache" || es.lastCallMethod != "hset" {
		t.Errorf("expected cache hset call, got %s.%s", es.lastCallFeature, es.lastCallMethod)
	}
	params, ok := es.lastCallParams.(map[string]any)
	if !ok {
		t.Fatalf("params should be map")
	}
	if params["key"] != "workflow-cache/myhash" {
		t.Errorf("unexpected hset key: %v", params["key"])
	}
	if params["ttl"] != "30m" {
		t.Errorf("unexpected hset ttl: %v", params["ttl"])
	}
	expected, _ := json.Marshal(map[string]any{"result": "data"})
	if !reflect.DeepEqual(params["value"], map[string]any{"myfield": string(expected)}) {
		t.Errorf("unexpected hset value: %v", params["value"])
	}
}

func TestProcessCheckCacheState_NormalKeyUsesLoad(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: false}
	es.callReturn = func(feature, method string) ([]byte, error) {
		if feature == "cache" && method == "load" {
			return []byte(`{"data": "normal-result"}`), nil
		}

		return nil, nil
	}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}

	s.processCheckCacheState()

	if es.lastCallFeature != "cache" || es.lastCallMethod != "load" {
		t.Errorf("expected cache load call for normal key, got %s.%s", es.lastCallFeature, es.lastCallMethod)
	}
	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("expected from_cache label to be set")
	}
}

func TestProcessSaveCacheState_NormalKeyUsesSave(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: true}
	s.store = es
	s.Workflow.CacheKey = "test-key"
	s.Workflow.CacheTTL = "1h"
	s.Ctx["cache-data"] = map[string]any{"result": "data"}

	s.processSaveCacheState()

	if es.lastCallFeature != "cache" || es.lastCallMethod != "save" {
		t.Errorf("expected cache save call for normal key, got %s.%s", es.lastCallFeature, es.lastCallMethod)
	}
	params, ok := es.lastCallParams.(map[string]any)
	if !ok {
		t.Fatalf("params should be map")
	}
	if params["key"] != "workflow-cache/test-key" {
		t.Errorf("unexpected cache key for normal key: %v", params["key"])
	}
}

func TestProcessCheckCacheState_WholeHashFallsThrough(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: false}
	es.callReturn = func(feature, method string) ([]byte, error) {
		if feature == "cache" && method == "load" {
			return []byte(`{"data": "whole-hash-result"}`), nil
		}

		return nil, nil
	}
	s.store = es
	s.Workflow.CacheKey = "myhash#"
	s.CurrentMsg = &dipper.Message{Labels: map[string]string{}}

	s.processCheckCacheState()

	if es.lastCallFeature != "cache" || es.lastCallMethod != "load" {
		t.Errorf("expected cache load call for whole-hash key (phase 3 not implemented), got %s.%s", es.lastCallFeature, es.lastCallMethod)
	}
	if s.CurrentMsg.Labels[LabelFromCache] != "true" {
		t.Error("expected from_cache label to be set")
	}
}

func TestProcessSaveCacheState_WholeHashFallsThrough(t *testing.T) {
	s := makeExecuteSession()
	es := &cacheTestStore{memcacheEnabled: true}
	s.store = es
	s.Workflow.CacheKey = "myhash#"
	s.Workflow.CacheTTL = "1h"
	s.Ctx["cache-data"] = map[string]any{"result": "data"}

	s.processSaveCacheState()

	if es.lastCallFeature != "cache" || es.lastCallMethod != "save" {
		t.Errorf("expected cache save call for whole-hash key (phase 3 not implemented), got %s.%s", es.lastCallFeature, es.lastCallMethod)
	}
}

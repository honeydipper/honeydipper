// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package service

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/internal/api"
	"github.com/honeydipper/honeydipper/internal/config"
	"github.com/honeydipper/honeydipper/internal/daemon"
	"github.com/honeydipper/honeydipper/internal/driver"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

const (
	// APIServerGracefulTimeout is the timeout in seconds for waiting for a http.Server to shutdown gracefully
	APIServerGracefulTimeout time.Duration = 10
)

var (
	// API is the service object for api service
	API *Service
	// APIServer is the http server listening for api calls
	APIServer *http.Server
	// APICfg contains configuration for the api service from driver.daemon.services.api
	APICfg interface{}
	// APIRequestStore is the object holds and handles all live api calls
	APIRequestStore *api.Store
)

// StartAPI starts the operator service.
func StartAPI(cfg *config.Config) {
	API = NewService(cfg, "api")
	API.ServiceReload = reloadAPI
	Services["api"] = API
	loadAPIConfig(cfg)
	APIRequestStore = api.NewStore(API)
	API.addResponder("eventbus:api", handleAPIMessage)
	API.DiscoverFeatures = APIFeatures
	startAPIListener()
	API.start()
}

// loadAPIConfig loads the API config and returns true if changed.
func loadAPIConfig(cfg *config.Config) bool {
	apiCfg, _ := dipper.GetMapData(cfg.DataSet.Drivers, "daemon.services.api")
	if !reflect.DeepEqual(apiCfg, APICfg) {
		APICfg = apiCfg
		return true
	}
	return false
}

// startAPIListener starts the web server to serve api requests.
func startAPIListener() {
	addr, ok := dipper.GetMapDataStr(APICfg, "listener.addr")
	if !ok {
		addr = ":9000"
	}
	APIServer = &http.Server{
		Addr: addr,
	}
	APIRequestStore.PrepareHTTPServer(APIServer, APICfg)
	go func() {
		dipper.Logger.Infof("[api] start listening for webhook requests")
		dipper.Logger.Warningf("[api] listener stopped: %+v", APIServer.ListenAndServe())
		if !daemon.ShuttingDown {
			startAPIListener()
		}
	}()
}

// reloadAPI reloads config and restarts the listener.
func reloadAPI(cfg *config.Config) {
	if loadAPIConfig(cfg) {
		ctx, cancel := context.WithTimeout(context.Background(), APIServerGracefulTimeout*time.Second)
		defer cancel()
		_ = APIServer.Shutdown(ctx)
	}
}

// handleAPIMessage handles messages from eventbus.
func handleAPIMessage(d *driver.Runtime, m *dipper.Message) {
	dipper.Logger.Debugf("[%s] handling eventbus:api message: %+v", d.Service, m.Labels)
	if t, ok := m.Labels["type"]; ok {
		switch t {
		case "ack":
			APIRequestStore.HandleAPIACK(m)
		case "result":
			APIRequestStore.HandleAPIReturn(m)
		}
	}
}

// APIFeatures figures out what features need to be loaded for API services.
func APIFeatures(s *config.DataSet) map[string]interface{} {
	features := map[string]interface{}{}
	if providers, ok := dipper.GetMapData(APICfg, "auth-providers"); ok && providers != nil {
		if len(providers.([]interface{})) > 0 {
			for _, p := range providers.([]interface{}) {
				parts := strings.Split(p.(string), ".")
				features["driver:"+parts[0]] = nil
			}
		}
	}
	return features
}

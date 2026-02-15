// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package service

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v3/internal/api"
	"github.com/honeydipper/honeydipper/v3/internal/config"
	"github.com/honeydipper/honeydipper/v3/internal/daemon"
	"github.com/honeydipper/honeydipper/v3/internal/driver"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
)

const (
	// APIServerGracefulTimeout is the timeout in seconds for waiting for a http.Server to shutdown gracefully.
	APIServerGracefulTimeout time.Duration = 10

	// RequestHeaderTimeoutSecs is the timeout (in seconds) for accepting incoming requests.
	RequestHeaderTimeoutSecs = 20
)

var (
	// API is the service object for api service.
	API *Service
	// APIServer is the http server listening for api calls.
	APIServer *http.Server
	// APICfg contains configuration for the api service from driver.daemon.services.api.
	APICfg interface{}
	// APIRequestStore is the object holds and handles all live api calls.
	APIRequestStore *api.Store
)

// StartAPI starts the operator service.
func StartAPI(cfg *config.Config) {
	API = NewService(cfg, "api")
	API.ServiceReload = reloadAPI
	API.DiscoverFeatures = APIFeatures
	API.addResponder("eventbus:api", handleAPIMessage)
	APIRequestStore = api.NewStore(API)
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
	prefix, ok := dipper.GetMapDataStr(APICfg, "api_prefix")
	if !ok || prefix == "" {
		prefix = "/api/"
	}
	healthcheckPrefix, ok := dipper.GetMapDataStr(APICfg, "healthcheck_prefix")
	if !ok || healthcheckPrefix == "" {
		healthcheckPrefix = "/healthz"
	}
	mux := http.NewServeMux()
	mux.Handle(prefix, APIRequestStore.GetAPIHandler(prefix, APICfg))
	mux.HandleFunc(healthcheckPrefix, func(w http.ResponseWriter, r *http.Request) {
		if API.CheckHealth() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	APIServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: RequestHeaderTimeoutSecs * time.Second,
	}
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
		if APIServer == nil {
			startAPIListener()
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), APIServerGracefulTimeout*time.Second)
			defer cancel()
			_ = APIServer.Shutdown(ctx)
		}
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
	if providers, ok := dipper.GetMapData(s.Drivers, "daemon.services.api.auth-providers"); ok && providers != nil {
		if len(providers.([]interface{})) > 0 {
			for _, p := range providers.([]interface{}) {
				parts := strings.Split(p.(string), ".")
				features["driver:"+parts[0]] = nil
			}
		}
	}

	return features
}

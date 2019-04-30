// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package webhook enables Honeydipper to receive incoming webhook requests.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
)

var log *logging.Logger

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports receiver service")
		fmt.Printf("  This program provides honeydipper with capability of receiving webhooks")
	}
}

var driver *dipper.Driver
var server *http.Server
var hooks map[string]interface{}

// Addr : listening address and port of the webhook
var Addr string

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "webhook")
	if driver.Service == "receiver" {
		driver.Start = startWebhook
		driver.Stop = stopWebhook
		driver.Reload = loadOptions
	}
	driver.Run()
}

func stopWebhook(*dipper.Message) {
	dipper.PanicError(server.Shutdown(context.Background()))
}

func loadOptions(m *dipper.Message) {
	log = driver.GetLogger()
	hooksObj, ok := driver.GetOption("dynamicData.collapsedEvents")
	if !ok {
		log.Panicf("[%s] no hooks defined for webhook driver", driver.Service)
	}
	hooks, ok = hooksObj.(map[string]interface{})
	if !ok {
		log.Panicf("[%s] hook data should be a map of event to conditions", driver.Service)
	}

	log.Debugf("[%s] hook data : %+v", driver.Service, hooks)

	NewAddr, ok := driver.GetOptionStr("data.Addr")
	if !ok {
		NewAddr = ":8080"
	}
	if driver.State == "alive" && NewAddr != Addr {
		stopWebhook(m) // the webhook will be restarted automatically in the loop
	}
	Addr = NewAddr
}

func startWebhook(m *dipper.Message) {
	loadOptions(m)
	server = &http.Server{
		Addr:    Addr,
		Handler: http.HandlerFunc(hookHandler),
	}
	go func() {
		log.Infof("[%s] start listening for webhook requests", driver.Service)
		log.Infof("[%s] listener stopped: %+v", driver.Service, server.ListenAndServe())
		if driver.State != "exit" && driver.State != "cold" {
			startWebhook(m)
		}
	}()
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
	eventData := extractEventData(w, r)

	matched := false
	for _, hook := range hooks {
		for _, condition := range hook.([]interface{}) {
			auth, ok := dipper.GetMapData(condition, ":auth:")
			if ok {
				authDriver := dipper.MustGetMapDataStr(auth, "driver")
				authResult, err := driver.RPCCall("driver:"+authDriver, "webhookAuth", map[string]interface{}{
					"event":     eventData,
					"condition": auth,
				})
				if err != nil || string(authResult) != "authenticated" {
					log.Warningf("[%s] failed to authenticate webhook request with %s error %+v", driver.Service, authDriver, err)
					continue
				}
			}
			if dipper.CompareAll(eventData, condition) {
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}

	if matched {
		driver.SendMessage(&dipper.Message{
			Channel: "eventbus",
			Subject: "message",
			Payload: map[string]interface{}{
				"events": []interface{}{"webhook."},
				"data":   eventData,
			},
		})

		w.WriteHeader(http.StatusOK)
		return
	}

	http.NotFound(w, r)
}

func badRequest(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	dipper.PanicError(io.WriteString(w, "Bad Request\n"))
}

func extractEventData(w http.ResponseWriter, r *http.Request) map[string]interface{} {
	dipper.PanicError(r.ParseForm())

	eventData := map[string]interface{}{
		"url":        r.URL.Path,
		"method":     r.Method,
		"form":       r.Form,
		"headers":    r.Header,
		"host":       r.Host,
		"remoteAddr": r.RemoteAddr,
	}

	log.Debugf("[%s] webhook event data: %+v", driver.Service, eventData)
	if r.Method == http.MethodPost {
		bodyBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			badRequest(w)
			log.Panicf("[%s] unable to read post body", driver.Service)
		}
		contentType := r.Header.Get("Content-type")
		eventData["body"] = string(bodyBytes)
		if len(contentType) > 0 && strings.HasPrefix(contentType, "application/json") {
			bodyObj := map[string]interface{}{}
			err := json.Unmarshal(bodyBytes, &bodyObj)
			if err != nil {
				badRequest(w)
				log.Panicf("[%s] invalid json in post body", driver.Service)
			}
			eventData["json"] = bodyObj
		}
	}

	return eventData
}

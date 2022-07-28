// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package webhook enables Honeydipper to receive incoming webhook requests.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
)

const (
	// RequestHeaderTimeoutSecs is the timeout (in seconds) for accepting incoming requests.
	RequestHeaderTimeoutSecs = 20
)

var log *logging.Logger

// ErrCalcHash is raised when unable to calculate the hash for validating the webhook request.
var ErrCalcHash = errors.New("unable to calculate the hash")

// ErrReplayAttack is raised when the timestamp of the request is not within 5 minutes.
var ErrReplayAttack = errors.New("replay attack detected")

// _SupportedSignatureHeaders is a list of supported signature headers.
var _SupportedSignatureHeaders = []string{
	"X-Hub-Signature-256",
	"X-PagerDuty-Signature",
	"X-Slack-Signature",
}

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports receiver service")
		fmt.Printf("  This program provides honeydipper with capability of receiving webhooks")
	}
}

var (
	driver *dipper.Driver
	server *http.Server
	hooks  map[string]interface{}
	sysMap map[string]map[string]interface{}
)

// Addr : listening address and port of the webhook.
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
	dipper.Must(server.Shutdown(context.Background()))
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

	sysMap = map[string]map[string]interface{}{}
	for _, hook := range hooks {
		for _, collapsed := range hook.([]interface{}) {
			rule := collapsed.(map[string]interface{})
			if sys, ok := rule["sysName"]; ok && sys.(string) != "" {
				sysMap[sys.(string)] = rule["sysData"].(map[string]interface{})
			}
		}
	}

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
		Addr:              Addr,
		Handler:           http.HandlerFunc(hookHandler),
		ReadHeaderTimeout: RequestHeaderTimeoutSecs * time.Second,
	}
	go func() {
		log.Infof("[%s] start listening for webhook requests", driver.Service)
		log.Infof("[%s] listener stopped: %+v", driver.Service, server.ListenAndServe())
		if driver.State != "stopped" && driver.State != "cold" {
			startWebhook(m)
		}
	}()
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
	eventData := extractEventData(w, r)
	if eventData == nil {
		return
	}

	defer func() {
		if e := recover(); e != nil {
			log.Warningf("Resuming after error: %v", e)
			log.Warning(errors.Wrap(e, 1).ErrorStack())
			w.WriteHeader(http.StatusInternalServerError)
			dipper.Must(io.WriteString(w, "Internal server error\n"))
		}
	}()

	if eventData["url"] == "/hz/alive" {
		w.WriteHeader(http.StatusOK)

		return
	}

	verifySystems(eventData)

	log.Debugf("[%s] webhook event data: %+v", driver.Service, eventData)
	matched := false
	for _, hook := range hooks {
		for _, collapsed := range hook.([]interface{}) {
			condition, _ := dipper.GetMapData(collapsed, "match")

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
		id := driver.EmitEvent(map[string]interface{}{
			"events": []interface{}{"webhook."},
			"data":   eventData,
		})

		if _, ok := dipper.GetMapDataStr(eventData, "form.accept_uuid.0"); ok {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf("{\"eventID\": \"%s\"}", id)))
		} else {
			w.WriteHeader(http.StatusOK)
		}

		return
	}

	http.NotFound(w, r)
}

func badRequest(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	dipper.Must(io.WriteString(w, "Bad Request\n"))
}

func extractEventData(w http.ResponseWriter, r *http.Request) map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			badRequest(w)
			log.Warningf("Resuming after error: %v", r)
			log.Warning(errors.Wrap(r, 1).ErrorStack())
		}
	}()

	return dipper.ExtractWebRequest(r)
}

func verifySignature(header, actual, secret string, eventData map[string]interface{}) bool {
	key := []byte(secret)
	mac := hmac.New(sha256.New, key)
	switch header {
	case "X-Slack-Signature":
		timestamp := eventData["headers"].(http.Header).Get("X-Slack-Request-Timestamp")
		if timestamp == "" {
			panic(ErrCalcHash)
		}
		if _, ok := eventData["skip_replay_check"]; !ok {
			current := time.Now().Unix()
			//nolint:gomnd
			requestedAt := dipper.Must(strconv.ParseInt(timestamp, 10, 64)).(int64)
			if current-requestedAt > 300 || current < requestedAt {
				panic(ErrReplayAttack)
			}
		}
		dipper.Must(mac.Write([]byte("v0:")))
		dipper.Must(mac.Write([]byte(timestamp)))
		dipper.Must(mac.Write([]byte(":")))
	case "X-PagerDuty-Signature":
	case "X-Hub-Signature-256": // github signature
	}
	dipper.Must(mac.Write(eventData["body"].([]byte)))
	expected := mac.Sum(nil)
	log.Infof("[%s] HMAC for %s calculated: %s", driver.Service, header, hex.EncodeToString(expected))

	var hashes []string
	switch header {
	case "X-Slack-Signature":
		hashes = []string{strings.TrimPrefix(actual, "v0=")}
	case "X-PagerDuty-Signature":
		for _, sig := range strings.Split(actual, ",") {
			hashes = append(hashes, strings.TrimPrefix(sig, "v1="))
		}
	case "X-Hub-Signature-256": // github signature
		hashes = []string{strings.TrimPrefix(actual, "sha256=")}
	}

	for _, hash := range hashes {
		if hmac.Equal(expected, dipper.Must(hex.DecodeString(hash)).([]byte)) {
			return true
		}
	}

	return false
}

func verifySystems(eventData map[string]interface{}) {
	headers := eventData["headers"].(http.Header)

	var signatureHeader, signatureValue string
	for _, supported := range _SupportedSignatureHeaders {
		signatureValue = headers.Get(supported)
		if signatureValue != "" {
			signatureHeader = supported

			break
		}
	}

	if signatureHeader == "" {
		return
	}

	var verifiedSystem []string
	for name, sys := range sysMap {
		expectedHeader, ok := sys["signatureHeader"]
		if !ok || !strings.EqualFold(expectedHeader.(string), signatureHeader) {
			// ignore unsupported headers or system without signatureHeader
			continue
		}

		secretValue, ok := dipper.GetMapData(sys, "signatureSecret")
		if !ok {
			log.Warningf("[%s] signature secret not defined for system %s", driver.Service, name)

			continue
		}
		secrets, ok := secretValue.([]interface{})
		if !ok {
			secrets = []interface{}{secretValue}
		}

		for _, secret := range secrets {
			if verifySignature(signatureHeader, signatureValue, secret.(string), eventData) {
				verifiedSystem = append(verifiedSystem, name)

				break
			}
		}
	}
	log.Infof("[%s] HMAC verified for system(s) %+v", driver.Service, verifiedSystem)
	eventData["verifiedSystem"] = verifiedSystem
}

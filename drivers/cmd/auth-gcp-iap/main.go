// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package auth-gcp-iap enables Honeydipper to authenticate/authorize incoming web requests through GCP IAP.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"google.golang.org/api/idtoken"
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports receiver and API service.")
		fmt.Printf("  This program provides honeydipper with the capability of authenticating the web request with gcloud IAP.")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "auth-gcp-iap")
	driver.RPCHandlers["auth_web_request"] = authWebRequest
	driver.Reload = func(*dipper.Message) {}
	driver.Run()
}

func authWebRequest(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	driver.GetLogger().Debugf("payloads are: %+v", m.Payload)
	token := dipper.InterpolateStr("$headers.X-Goog-Iap-Jwt-Assertion.0,headers.x-goog-iap-jwt-assertion.0", m.Payload)
	audience := dipper.MustGetMapDataStr(driver.Options, "data.audience")

	payload := dipper.Must(idtoken.Validate(context.Background(), token, audience)).(*idtoken.Payload)
	driver.GetLogger().Debugf("claims are: %+v", payload.Claims)
	subject := dipper.MustGetMapDataStr(payload.Claims, "email")

	m.Reply <- dipper.Message{
		Payload: subject,
	}
}

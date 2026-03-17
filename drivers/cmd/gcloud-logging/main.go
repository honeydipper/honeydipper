// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package gcloud-logging enables Honeydipper to send logs to GCP natively.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/logging"
	"github.com/honeydipper/honeydipper/v3/pkg/dipper"
	"google.golang.org/api/option"
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports the operator service.")
		fmt.Printf("  This program provides honeydipper with capability of sending logs to GCP using logging API.")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "google-logging")
	if driver.Service == "operator" {
		driver.Reload = func(*dipper.Message) {}
		driver.Commands["log"] = sendLog
		driver.Run()
	}
}

func sendLog(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	severity, _ := dipper.GetMapDataStr(m.Payload, "severity")
	loggerPath := dipper.MustGetMapDataStr(m.Payload, "logger")
	logger := getGCPLogger(loggerPath)
	logger.Log(logging.Entry{Severity: logging.ParseSeverity(severity), Payload: dipper.MustGetMapData(m.Payload, "payload")})
	m.Reply <- dipper.Message{}
}

func getGCPLogger(loggerPath string) *logging.Logger {
	l, ok := driver.GetOption("_runtime.loggers." + loggerPath)
	if !ok {
		var loggerName, parent string
		//nolint:gomnd
		parts := strings.SplitN(loggerPath, "|", 2)
		//nolint:gomnd
		if len(parts) < 2 {
			loggerName = strings.TrimSpace(parts[0])
			parent, _ = metadata.ProjectIDWithContext(context.Background())
		} else {
			parent, loggerName = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}

		options := []option.ClientOption{}
		serviceAccount, _ := driver.GetOptionStr("loggers." + loggerPath + ".service_account")
		if serviceAccount != "" {
			options = append(options, option.WithCredentialsJSON([]byte(serviceAccount)))
		}
		client := dipper.Must(logging.NewClient(context.Background(), parent, options...)).(*logging.Client)
		l = client.Logger(loggerName)

		delta := map[string]interface{}{"_runtime": map[string]interface{}{"loggers": map[string]interface{}{loggerPath: l}}}
		if driver.Options == nil {
			driver.Options = map[string]interface{}{}
		}
		driver.Options = dipper.CombineMap(driver.Options.(map[string]interface{}), delta)
	}

	return l.(*logging.Logger)
}

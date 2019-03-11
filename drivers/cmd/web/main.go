// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package web enables Honeydipper to make outbound web requests.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
)

var log *logging.Logger

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("  This driver supports operator service\n")
		fmt.Printf("  This driver is used for Honeydipper to make outgoing web requests\n")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "web")
	if driver.Service == "operator" {
		driver.Reload = func(*dipper.Message) {
			log = nil
		} // allow hot reload
		driver.CommandProvider.Commands["request"] = sendRequest
		driver.Run()
	}
}

func sendRequest(m *dipper.Message) {
	if log == nil {
		log = driver.GetLogger()
	}
	m = dipper.DeserializePayload(m)
	rurl, ok := dipper.GetMapDataStr(m.Payload, "URL")
	if !ok {
		log.Panicf("[%s] URL is required but missing", driver.Service)
	}

	var form = url.Values{}
	formData, _ := dipper.GetMapData(m.Payload, "form")
	if formData != nil {
		for k, v := range formData.(map[string]interface{}) {
			form.Add(k, v.(string))
		}
	}

	var header = http.Header{}
	headerData, _ := dipper.GetMapData(m.Payload, "header")
	if headerData != nil {
		for k, v := range headerData.(map[string]interface{}) {
			header.Set(k, v.(string))
		}
	}

	method, ok := dipper.GetMapDataStr(m.Payload, "method")
	if !ok {
		method = "GET"
	}

	var req *http.Request
	var err error
	if method == "POST" || method == "PUT" {
		content, ok := dipper.GetMapData(m.Payload, "content")
		if ok {
			switch v := content.(type) {
			case string:
				req, err = http.NewRequest(method, rurl, bytes.NewBufferString(v))
				if err != nil {
					panic(err)
				}
			case map[string]interface{}:
				if strings.HasPrefix(header.Get("content-type"), "application/json") {
					contentBytes, err := json.Marshal(v)
					if err != nil {
						panic(err)
					}
					req, err = http.NewRequest(method, rurl, bytes.NewBuffer(contentBytes))
					if err != nil {
						panic(err)
					}
				} else {
					var postForm url.Values
					for key, val := range v {
						postForm.Add(key, val.(string))
					}
					contentStr := postForm.Encode()
					req, err = http.NewRequest(method, rurl, bytes.NewBufferString(contentStr))
					if err != nil {
						panic(err)
					}
				}
			default:
				log.Panic("Unable to handle the content")
			}
		} else {
			if strings.HasPrefix(header.Get("content-type"), "application/json") {
				contentBytes, err := json.Marshal(formData)
				if err != nil {
					panic(err)
				}
				req, err = http.NewRequest(method, rurl, bytes.NewBuffer(contentBytes))
				if err != nil {
					panic(err)
				}
			} else {
				contentStr := form.Encode()
				req, err = http.NewRequest(method, rurl, bytes.NewBufferString(contentStr))
				if err != nil {
					panic(err)
				}
			}
		}
	} else {
		req, err = http.NewRequest(method, rurl, nil)
		if err != nil {
			panic(err)
		}
		if len(req.URL.RawQuery) > 0 {
			req.URL.RawQuery = req.URL.RawQuery + "&" + form.Encode()
		} else {
			req.URL.RawQuery = form.Encode()
		}
	}

	req.Header = header

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		defer resp.Body.Close()
	}

	response := extractHTTPResponseData(resp)
	statusCode, _ := strconv.Atoi(response["status_code"].(string))
	ret := dipper.Message{
		Payload: response,
		IsRaw:   false,
	}
	if statusCode >= 400 {
		ret.Labels = map[string]string{
			"error": fmt.Sprintf("Error: got status code: %d", statusCode),
		}
	}

	m.Reply <- ret
}

func extractHTTPResponseData(r *http.Response) map[string]interface{} {
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Panicf("[%s] unable to read resp body", driver.Service)
	}

	var cookies = map[string]interface{}{}
	for _, c := range r.Cookies() {
		cookies[c.Name] = c.Value
	}

	respData := map[string]interface{}{
		"status_code": strconv.Itoa(r.StatusCode),
		"cookies":     cookies,
		"headers":     r.Header,
		"body":        string(bodyBytes),
	}

	contentType := r.Header.Get("Content-type")
	if len(contentType) > 0 && strings.HasPrefix(contentType, "application/json") {
		bodyObj := map[string]interface{}{}
		err := json.Unmarshal(bodyBytes, &bodyObj)
		if err != nil {
			log.Panicf("[%s] invalid json in response body", driver.Service)
		}
		respData["json"] = bodyObj
	}

	log.Debugf("[%s] web response data: %+v", driver.Service, respData)

	return respData
}

// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package web enables Honeydipper to make outbound web requests.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
		driver.Commands["request"] = sendRequest
		driver.Run()
	}
}

func prepareRequest(m *dipper.Message) *http.Request {
	rurl, ok := dipper.GetMapDataStr(m.Payload, "URL")
	if !ok {
		log.Panicf("[%s] URL is required but missing", driver.Service)
	}

	form := url.Values{}
	formData, _ := dipper.GetMapData(m.Payload, "form")
	if formData != nil {
		for k, v := range formData.(map[string]interface{}) {
			switch val := v.(type) {
			case string:
				form.Add(k, val)
			case []interface{}:
				for _, vs := range val {
					form.Add(k, vs.(string))
				}
			}
		}
	}

	header := http.Header{}
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

	return createRequest(method, rurl, header, form, m)
}

func prepareRequestBody(form url.Values, header http.Header, m *dipper.Message) io.Reader {
	var buf io.Reader

	content, hasContent := dipper.GetMapData(m.Payload, "content")
	contentStr, contentIsStr := content.(string)
	needsJSON := strings.HasPrefix(header.Get("content-type"), "application/json")

	switch {
	case contentIsStr:
		buf = bytes.NewBufferString(contentStr)
	case hasContent && needsJSON:
		contentBytes := dipper.Must(json.Marshal(content)).([]byte)
		buf = bytes.NewBuffer(contentBytes)
	case hasContent:
		postForm := url.Values{}
		for key, val := range content.(map[string]interface{}) {
			postForm.Add(key, val.(string))
		}
		buf = bytes.NewBufferString(postForm.Encode())
	case needsJSON:
		formData, _ := dipper.GetMapData(m.Payload, "form")
		contentBytes := dipper.Must(json.Marshal(formData)).([]byte)
		buf = bytes.NewBuffer(contentBytes)
	default:
		buf = bytes.NewBufferString(form.Encode())
	}

	return buf
}

func createRequest(method, rurl string, header http.Header, form url.Values, m *dipper.Message) *http.Request {
	var req *http.Request

	switch method {
	case "POST":
		fallthrough
	case "PUT":
		buf := prepareRequestBody(form, header, m)
		req = dipper.Must(http.NewRequest(method, rurl, buf)).(*http.Request)
	default: // GET
		req = dipper.Must(http.NewRequest(method, rurl, nil)).(*http.Request)
		if len(req.URL.RawQuery) > 0 {
			req.URL.RawQuery += "&"
		}
		req.URL.RawQuery += form.Encode()
	}

	req.Header = header

	return req
}

func sendRequest(m *dipper.Message) {
	if log == nil {
		log = driver.GetLogger()
	}
	m = dipper.DeserializePayload(m)
	req := prepareRequest(m)

	client := http.Client{}
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		panic(err)
	}

	response := extractHTTPResponseData(resp)
	statusCode, _ := strconv.Atoi(response["status_code"].(string))
	ret := dipper.Message{
		Payload: response,
		IsRaw:   false,
	}
	if statusCode >= http.StatusBadRequest {
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

	cookies := map[string]interface{}{}
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
	if len(contentType) > 0 && strings.Contains(contentType, "json") {
		var bodyObj interface{}
		err := json.Unmarshal(bodyBytes, &bodyObj)
		if err != nil {
			log.Panicf("[%s] invalid json in response body", driver.Service)
		}
		respData["json"] = bodyObj
	}

	log.Debugf("[%s] web response data: %+v", driver.Service, respData)

	return respData
}

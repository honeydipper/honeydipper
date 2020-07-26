// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

package dipper

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
)

// ExtractWebRequest put needed information from a request in a map.
func ExtractWebRequest(r *http.Request) map[string]interface{} {
	PanicError(r.ParseForm())

	req := map[string]interface{}{
		"url":        r.URL.Path,
		"method":     r.Method,
		"form":       r.Form,
		"headers":    r.Header,
		"host":       r.Host,
		"remoteAddr": r.RemoteAddr,
	}

	if r.Method == http.MethodPost {
		req["body"] = Must(ioutil.ReadAll(r.Body))
		if strings.HasPrefix(r.Header.Get("content-type"), "application/json") {
			bodyObj := map[string]interface{}{}
			PanicError(json.Unmarshal(req["body"].([]byte), &bodyObj))
			req["json"] = bodyObj
		}
	}

	return req
}

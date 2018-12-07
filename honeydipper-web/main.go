package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/op/go-logging"
	"net/http"
	"net/url"
	"os"
)

var log *logging.Logger = dipper.GetLogger("web")

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service")
		fmt.Printf("  This program provides honeydipper with capability of access webhooks")
	}
}

var driver *dipper.Driver

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "web")
	if driver.Service == "operator" {
		driver.Reload = func(*dipper.Message) {} // allow hot reload
		driver.MessageHandlers["execute:request"] = sendRequest
		driver.Run()
	}
}

func sendRequest(m *dipper.Message) {
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
				if header.Get("content-type") == "application/json" {
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
			if header.Get("content-type") == "application/json" {
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
	log.Infof("%+v, %+v", resp, err)
	// to do return values
	if sessionID, ok := m.Labels["sessionID"]; ok && sessionID != "" {
		retMsg := &dipper.Message{
			Channel: "eventbus",
			Subject: "return",
			Labels:  m.Labels,
		}

		retMsg.Labels["status"] = "success"
		if err != nil {
			retMsg.Labels["status"] = "failure"
			retMsg.Labels["reason"] = "some error"
		}

		driver.SendMessage(retMsg)
	}
}

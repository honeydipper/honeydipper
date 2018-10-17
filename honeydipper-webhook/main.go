package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports receiver service")
		fmt.Printf("  This program provides honeydipper with capability of receiving webhooks")
	}
}

var driver *dipper.Driver
var ok bool
var server *http.Server
var hooks map[string]map[string]interface{}

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "webhook")
	if driver.Service == "receiver" {
		driver.Start = startWebhook
		driver.Reload = stopWebhook
	}
	driver.Run()
}

func stopWebhook(*dipper.Message) {
	server.Shutdown(context.Background())
}

func startWebhook(m *dipper.Message) {
	Addr, ok := driver.GetOptionStr("Addr")
	if !ok {
		Addr = ":8080"
	}
	systems, ok := driver.GetOption("Systems")
	if !ok {
		log.Panicf("[%s-%s] no system defined")
	}
	hooks = map[string]map[string]interface{}{}
	for system, events := range systems.(map[string]interface{}) {
	NextEvent:
		for event, hook := range events.(map[string]interface{}) {
			if pattern, ok := dipper.GetMapDataStr(hook, "pattern"); ok {
				re, err := regexp.Compile(pattern)
				if err != nil {
					log.Printf("[%s-%s] skipping invalid pattern %s in %s.%s", driver.Service, driver.Name, pattern, system, event)
					break NextEvent
				}
				hook.(map[string]interface{})["pattern"] = re
			}
			hooks[system+"."+event] = hook.(map[string]interface{})
		}
	}
	server = &http.Server{
		Addr:    Addr,
		Handler: http.HandlerFunc(hookHandler),
	}
	go func() {
		log.Printf("[%s-%s] start listening for webhook requests", driver.Service, driver.Name)
		log.Printf("[%s-%s] listener stopped: %+v", driver.Service, driver.Name, server.ListenAndServe())
		if driver.State != "quiting" && driver.State != "cold" {
			startWebhook(m)
		}
	}()
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
	matched := []string{}
	r.ParseForm()
NextEntry:
	for SystemEvent, hook := range hooks {
		meet := true
		for check, value := range hook {
			switch check {
			case "auth_token":
				if r.Form.Get("auth_token") != value.(string) {
					meet = false
					break NextEntry
				}
			case "token":
				if r.Form.Get("token") != value.(string) {
					meet = false
					break NextEntry
				}
			case "pattern":
				if !value.(*regexp.Regexp).MatchString(r.URL.Path) {
					meet = false
					break NextEntry
				}
			case "remoteaddr":
				if r.RemoteAddr != value.(string) {
					meet = false
					break NextEntry
				}
			case "method":
				if r.Method != value.(string) {
					meet = false
					break NextEntry
				}
			}
		}

		if meet {
			matched = append(matched, SystemEvent)
		}
	}

	if len(matched) > 0 {
		payload := map[string]interface{}{
			"events":     matched,
			"url":        r.URL.Path,
			"method":     r.Method,
			"form":       r.Form.Encode(),
			"headers":    r.Header,
			"remoteaddr": r.RemoteAddr,
		}

		if r.Method == http.MethodPost {
			bodyBytes, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("[%s-%s] unable to read post body", driver.Service, driver.Name)
				badRequest(w, r)
				return
			}
			contentType := r.Header.Get("Content-type")
			if len(contentType) > 0 && strings.EqualFold(contentType, "application/json") {
				bodyObj := map[string]interface{}{}
				err := json.Unmarshal(bodyBytes, bodyObj)
				payload["body"] = bodyObj
				if err != nil {
					log.Printf("[%s-%s] invalid json in post body", driver.Service, driver.Name)
					badRequest(w, r)
					return
				}
			} else {
				payload["body"] = string(bodyBytes)
			}
		}

		driver.SendMessage("eventbus", "message", payload)

		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "Done\n")
		return
	}

	http.NotFound(w, r)
}

func badRequest(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	io.WriteString(w, "Bad Request\n")
}

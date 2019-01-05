package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/op/go-logging"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

var log *logging.Logger

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
var hooks map[string]interface{}

// Addr : listening address and port of the webhook
var Addr string

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "webhook")
	log = driver.GetLogger()
	if driver.Service == "receiver" {
		driver.Start = startWebhook
		driver.Stop = stopWebhook
		driver.Reload = loadOptions
	}
	driver.Run()
}

func stopWebhook(*dipper.Message) {
	server.Shutdown(context.Background())
}

func loadOptions(m *dipper.Message) {
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
		io.WriteString(w, "Done\n")
		return
	}

	http.NotFound(w, r)
}

func badRequest(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	io.WriteString(w, "Bad Request\n")
}

func extractEventData(w http.ResponseWriter, r *http.Request) map[string]interface{} {
	r.ParseForm()

	eventData := map[string]interface{}{
		"url":     r.URL.Path,
		"method":  r.Method,
		"form":    r.Form,
		"headers": r.Header,
	}

	log.Debugf("[%s] webhook event data: %+v", driver.Service, eventData)
	if r.Method == http.MethodPost {
		bodyBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			badRequest(w, r)
			log.Panicf("[%s] unable to read post body", driver.Service)
		}
		contentType := r.Header.Get("Content-type")
		eventData["body"] = string(bodyBytes)
		if len(contentType) > 0 && strings.HasPrefix(contentType, "application/json") {
			bodyObj := map[string]interface{}{}
			err := json.Unmarshal(bodyBytes, &bodyObj)
			if err != nil {
				badRequest(w, r)
				log.Panicf("[%s] invalid json in post body", driver.Service)
			}
			eventData["json"] = bodyObj
		}
	}

	return eventData
}

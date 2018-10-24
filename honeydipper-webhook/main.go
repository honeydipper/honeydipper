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
	"net/url"
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
var hooks map[string]interface{}

// Addr : listening address and port of the webhook
var Addr string

func main() {
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
	server.Shutdown(context.Background())
}

func loadOptions(m *dipper.Message) {
	systems, ok := driver.GetOption("dynamicData")
	if !ok {
		log.Panicf("[%s-%s] no system defined", driver.Service, driver.Name)
	}
	systemMap, ok := systems.(map[string]interface{})
	if !ok {
		log.Panicf("[%s-%s] systems should be a map", driver.Service, driver.Name)
	}

	hooks = map[string]interface{}{}
	for system, events := range systemMap {
		eventMap, ok := events.(map[string]interface{})
		if !ok {
			log.Panicf("[%s-%s] every system should map to a list of events", driver.Service, driver.Name)
		}
		for event, definition := range eventMap {
			hooks[system+"."+event] = definition.(map[string]interface{})
		}
	}

	dipper.Recursive(hooks, func(key string, val interface{}) (ret interface{}, ok bool) {
		if str, ok := val.(string); ok {
			if strings.HasPrefix(str, ":regex:") {
				if newval, err := regexp.Compile(str[7:]); err == nil {
					return newval, true
				}
				log.Printf("[%s-%s] skipping invalid regex pattern %s", driver.Service, driver.Name, str[7:])
			}
			return nil, false
		}
		str := fmt.Sprintf("%v", val)
		return str, true
	})

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
		log.Printf("[%s-%s] start listening for webhook requests", driver.Service, driver.Name)
		log.Printf("[%s-%s] listener stopped: %+v", driver.Service, driver.Name, server.ListenAndServe())
		if driver.State != "exit" && driver.State != "cold" {
			startWebhook(m)
		}
	}()
}

func compare(left string, right interface{}) bool {
	strVal, ok := right.(string)
	if ok {
		return (strVal == left)
	}

	re, ok := right.(*regexp.Regexp)
	if ok {
		return re.Match([]byte(left))
	}

	listVal, ok := right.([]interface{})
	if ok {
		for _, subVal := range listVal {
			if compare(left, subVal) {
				return true
			}
		}
	}
	return false
}

func checkForm(form url.Values, values interface{}) bool {
	formChecks, ok := values.(map[string]interface{})
	if !ok {
		return false
	}
	for field, value := range formChecks {
		actual := form.Get(field)
		if !compare(actual, value) {
			return false
		}
	}
	return true
}

func checkHeader(headers http.Header, values interface{}) bool {
	headerChecks, ok := values.(map[string]interface{})
	if !ok {
		return false
	}
	for header, expected := range headerChecks {
		actual := headers.Get(header)
		if !compare(actual, expected) {
			return false
		}
	}
	return true
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
	matched := []string{}
	r.ParseForm()
	for SystemEvent, hook := range hooks {
		meet := true
		for check, value := range hook.(map[string]interface{}) {
			if check == "url" {
				meet = compare(r.URL.Path, value)
			} else if check == "form" {
				meet = checkForm(r.Form, value)
			} else if check == "header" {
				meet = checkHeader(r.Header, value)
			} else if check == "method" {
				meet = compare(r.Method, value)
			} else {
				meet = false
			}

			if !meet {
				break
			}
		}

		if meet {
			matched = append(matched, SystemEvent)
		}
	}

	if len(matched) > 0 {
		eventData := map[string]interface{}{
			"url":     r.URL.Path,
			"method":  r.Method,
			"form":    r.Form.Encode(),
			"headers": r.Header,
		}

		if r.Method == http.MethodPost {
			bodyBytes, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("[%s-%s] unable to read post body", driver.Service, driver.Name)
				badRequest(w, r)
				return
			}
			contentType := r.Header.Get("Content-type")
			eventData["body"] = string(bodyBytes)
			if len(contentType) > 0 && strings.EqualFold(contentType, "application/json") {
				bodyObj := map[string]interface{}{}
				err := json.Unmarshal(bodyBytes, bodyObj)
				eventData["json"] = bodyObj
				if err != nil {
					log.Printf("[%s-%s] invalid json in post body", driver.Service, driver.Name)
					badRequest(w, r)
					return
				}
			}
		}

		payload := map[string]interface{}{
			"events": matched,
			"data":   eventData,
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

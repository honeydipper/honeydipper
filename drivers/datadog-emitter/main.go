package main

import (
	"flag"
	"fmt"
	"github.com/DataDog/datadog-go/statsd"
	"github.com/honeyscience/honeydipper/dipper"
	"github.com/mitchellh/mapstructure"
	"github.com/op/go-logging"
	"os"
	"strconv"
)

// DatadogOptions : datadog statsd connection options
type DatadogOptions struct {
	UseHostPort bool
	StatsdHost  string
	StatsdPort  string
}

func init() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc")
		fmt.Printf("  This program emits honeydipper internal metrics to datadog")
	}
}

var driver *dipper.Driver
var log *logging.Logger
var datadogOptions DatadogOptions
var dogstatsd *statsd.Client

func main() {
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "datadog-emitter")
	log = driver.GetLogger()
	driver.RPC.Provider.RPCHandlers["counter_increment"] = counterIncr
	driver.RPC.Provider.RPCHandlers["gauge_set"] = gaugeSet
	driver.Reload = loadOptions
	driver.Start = loadOptions
	driver.Run()
}

func loadOptions(msg *dipper.Message) {
	ddOptions, ok := driver.GetOption("data")
	if !ok {
		log.Panicf("datadog options not found")
	}
	datadogOptions = DatadogOptions{}
	err := mapstructure.Decode(ddOptions, &datadogOptions)
	if err != nil {
		panic(err)
	}
	if datadogOptions.UseHostPort {
		var ok bool
		if datadogOptions.StatsdHost, ok = os.LookupEnv("DOGSTATSD_HOST_IP"); !ok {
			log.Panicf("datadog host IP not set")
		}
	}

	if dogstatsd != nil {
		dogstatsd.Close()
	}
	dogstatsd, err = statsd.New(datadogOptions.StatsdHost + ":" + datadogOptions.StatsdPort)
	if err != nil {
		panic(err)
	}

	dogstatsd.Event(&statsd.Event{
		Title: "Honeydipper statistics started",
		Text:  "Honeydipper statistics started",
	})
}

func counterIncr(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload.(map[string]interface{})
	name := params["name"].(string)
	tagsObj := params["tags"].([]interface{})
	tags := []string{}
	for _, tag := range tagsObj {
		tags = append(tags, tag.(string))
	}

	dogstatsd.Incr(name, tags, 1)
}

func gaugeSet(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	params := msg.Payload.(map[string]interface{})
	name := params["name"].(string)
	value, err := strconv.ParseFloat(params["value"].(string), 64)
	if err != nil {
		panic(err)
	}
	tagsObj := params["tags"].([]interface{})
	tags := []string{}
	for _, tag := range tagsObj {
		tags = append(tags, tag.(string))
	}

	dogstatsd.Gauge(name, value, tags, 1)
}

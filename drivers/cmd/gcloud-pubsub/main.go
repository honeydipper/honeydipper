package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"google.golang.org/api/option"
)

var (
	// ErrFailedCreateClient means failure during create client
	ErrFailedCreateClient = errors.New("unable to create gcloud pubsub client")
)

// SubscriberConfig stores all gcloud pubsub subscriber information.
type SubscriberConfig struct {
	Project          string
	ServiceAccount   string
	SubscriptionName string
	Conditions       []interface{}
}

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports all services including engine, receiver, workflow, operator etc\n")
		fmt.Printf("  This program provides honeydipper with capability of interacting with gcloud pubsub\n")
	}
}

var driver *dipper.Driver
var subscriberConfigs map[string]*SubscriberConfig
var serviceAccount string

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "gcloud-pubsub")
	driver.Start = start
	driver.Run()
}

func getPubsubClient(serviceAccountBytes, project string) *pubsub.Client {
	var (
		client *pubsub.Client
		err    error
	)
	if len(serviceAccountBytes) > 0 {
		clientOption := option.WithCredentialsJSON([]byte(serviceAccountBytes))
		client, err = pubsub.NewClient(context.Background(), project, clientOption)
	} else {
		client, err = pubsub.NewClient(context.Background(), project)
	}
	if err != nil {
		panic(ErrFailedCreateClient)
	}
	return client
}

func loadOptions() {
	events, ok := driver.GetOption("dynamicData.collapsedEvents")
	dipper.Logger.Debugf("[%s] pubsub events %+v", driver.Service, events)
	if !ok {
		dipper.Logger.Panicf("[%s] no pubsub subscription defined for pubsub driver", driver.Service)
	}
	pubsubEvents, ok := events.(map[string]interface{})
	if !ok {
		dipper.Logger.Panicf("[%s] pubsub subscription data should be a map of event to conditions", driver.Service)
	}

	serviceAccount, ok = driver.GetOptionStr("data.service_account")
	if !ok {
		dipper.Logger.Warningf("[%s] doesn't find service account in driver data", driver.Service)
	}

	subscriberConfigs = map[string]*SubscriberConfig{}
	for _, pubsubEvent := range pubsubEvents {
		for _, branch := range pubsubEvent.([]interface{}) {
			condition := branch.(map[string]interface{})["match"]
			project, ok := dipper.GetMapDataStr(condition, "project")
			if !ok {
				dipper.Logger.Warningf("[%s] failed to get pubsub project in condition", driver.Service)
				continue
			}
			subscriptionName, ok := dipper.GetMapDataStr(condition, "subscriptionName")
			if !ok {
				dipper.Logger.Warningf("[%s] failed to get pubsub subscription name in condition", driver.Service)
				continue
			}

			subscriberName := project + ":" + subscriptionName
			subscriberConfig, ok := subscriberConfigs[subscriberName]
			if !ok {
				subscriberConfigs[subscriberName] = &SubscriberConfig{
					Project:          project,
					SubscriptionName: subscriptionName,
					Conditions: []interface{}{
						condition,
					},
				}
			} else {
				subscriberConfig.Conditions = append(
					subscriberConfig.Conditions,
					condition,
				)
			}
		}
	}
}

func start(msg *dipper.Message) {
	loadOptions()
	go subscribeAll()
}

type msgHandler func(ctx context.Context, msg *pubsub.Message)

func msgHandlerBuilder(config *SubscriberConfig) msgHandler {
	ret := func(ctx context.Context, msg *pubsub.Message) {
		project := config.Project
		subscriptionName := config.SubscriptionName
		conditions := config.Conditions
		actual := map[string]interface{}{
			"project":          project,
			"subscriptionName": subscriptionName,
		}

		var data interface{}
		err := json.Unmarshal(msg.Data, &data)
		if err != nil {
			actual["text"] = string(msg.Data)
		} else {
			actual["json"] = data
		}

		dipper.Logger.Debugf("[%s] pubsub payload: %+v", driver.Service, actual)

		matched := false
		for _, subCond := range conditions {
			if dipper.CompareAll(actual, subCond) {
				matched = true
				break
			}
		}

		if matched {
			driver.EmitEvent(map[string]interface{}{
				"events": []interface{}{"gcloud-pubsub."},
				"data":   actual,
			})
		} else {
			dipper.Logger.Debugf("Incoming message [%v] does not match with any gcloud-pubsub subscriber rule", data)
		}
	}

	return ret
}

func subscribeAll() {
	dipper.Logger.Debugf("[%s] subscribing: %+v", driver.Service, subscriberConfigs)

	for _, subscriberConfig := range subscriberConfigs {
		go func(config *SubscriberConfig) {
			for {
				func() {
					project := config.Project
					subscriptionName := config.SubscriptionName

					defer dipper.SafeExitOnError("[%s] re-subscribing to gcloud pubsub %s", driver.Service, subscriptionName)
					client := getPubsubClient(serviceAccount, project)

					defer client.Close()
					sub := client.Subscription(subscriptionName)

					msgFunc := msgHandlerBuilder(config)
					err := sub.Receive(context.Background(), func(ctx context.Context, msg *pubsub.Message) {
						msgFunc(ctx, msg)
						msg.Ack()
					})
					if !errors.Is(err, context.Canceled) {
						dipper.Logger.Warningf("Failed to receive message from pubsub [%s]", subscriptionName)
					}
				}()
				time.Sleep(time.Second)
			}
		}(subscriberConfig)
	}
}

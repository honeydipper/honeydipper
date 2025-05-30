// Copyright 2022 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package dipper

import (
	"context"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/op/go-logging"
)

const (
	// DriverLogDescriptor is the file descriptor used for logging in driver,
	// since the daemon always pass the log file descriptor as the first item in
	// the ExtraFiles list, this is always 3.
	DriverLogDescriptor uintptr = 3

	// DefaultAPITimeout is the default timeout for making an outbound API call.
	DefaultAPITimeout time.Duration = 10

	// DriverStateCompleted indicates a driver can be gracefully shutdown. Currently,
	// only used in tests.
	DriverStateCompleted = "completed"
)

// Driver : the helper stuct for creating a honey-dipper driver in golang.
type Driver struct {
	RPCCallerBase
	RPCProvider
	CommandProvider
	Name            string
	Service         string
	State           string
	In              io.Reader
	Out             io.Writer
	Options         interface{}
	MessageHandlers map[string]MessageHandler
	Start           MessageHandler
	Stop            MessageHandler
	Reload          MessageHandler
	ReadySignal     chan bool
	APITimeout      time.Duration
}

// DriverOption provides a way to pass parameters to NewDriver method to override
// default settings; useful for writing tests.
type DriverOption func(*Driver)

// DriverWithReader overrides the drivers input stream so mock message can be injected.
func DriverWithReader(r io.Reader) DriverOption {
	return func(d *Driver) {
		d.In = r
	}
}

// DriverWithWriter overrides the driver output stream so output can be intercepted.
func DriverWithWriter(w io.Writer) DriverOption {
	return func(d *Driver) {
		d.Out = w
	}
}

// NewDriver : create a blank driver object.
func NewDriver(service string, name string, opts ...DriverOption) *Driver {
	driver := Driver{
		Name:        name,
		Service:     service,
		State:       "loaded",
		In:          os.Stdin,
		Out:         os.Stdout,
		ReadySignal: make(chan bool),
	}

	for _, opt := range opts {
		opt(&driver)
	}

	driver.RPCProvider.Init("rpc", "return", driver.Out)
	driver.RPCCallerBase.Init(&driver, "rpc", "call")
	driver.CommandProvider.Init("eventbus", "return", driver.Out)

	driver.MessageHandlers = map[string]MessageHandler{
		"command:options":  driver.ReceiveOptions,
		"command:ping":     driver.Ping,
		"command:start":    driver.start,
		"command:stop":     driver.stop,
		"rpc:call":         driver.RPCProvider.Router,
		"rpc:return":       driver.HandleReturn,
		"eventbus:command": driver.CommandProvider.Router,
	}

	driver.GetLogger()

	return &driver
}

// Run : start a loop to communicate with daemon.
func (d *Driver) Run() {
	Logger.Infof("[%s] driver loaded", d.Service)
	for {
		func() {
			defer SafeExitOnError("[%s] Resuming driver message loop", d.Service)
			defer CatchError(io.EOF, func() {
				if d.State != DriverStateCompleted { // allow graceful shutdown during testing.
					Logger.Fatalf("[%s] daemon closed channel", d.Service)
				}
			})
			for {
				msg := FetchRawMessage(d.In)
				go func() {
					defer SafeExitOnError("[%s] Continuing driver message loop", d.Service)
					if handler, ok := d.MessageHandlers[msg.Channel+":"+msg.Subject]; ok {
						handler(msg)
					} else {
						Logger.Infof("[%s] skipping message without handler: %s:%s", d.Service, msg.Channel, msg.Subject)
					}
				}()
			}
		}()
		// allow graceful shutdown during testing.
		if d.State == DriverStateCompleted {
			break
		}
	}
}

// Ping : respond to daemon ping request with driver state.
func (d *Driver) Ping(msg *Message) {
	d.SendMessage(&Message{
		Channel: "state",
		Subject: d.State,
	})
}

// ReceiveOptions : receive options from daemon.
func (d *Driver) ReceiveOptions(msg *Message) {
	msg = DeserializePayload(msg)
	Recursive(msg.Payload, RegexParser)
	DecryptAll(d, msg.Payload)
	d.Options = msg.Payload
	Logger = nil
	d.GetLogger()
	d.APITimeout = DefaultAPITimeout
	apiTimeoutStr, ok := d.GetOptionStr("data.api_timeout")
	if ok {
		apiTimeout, e := strconv.Atoi(apiTimeoutStr)
		if e != nil {
			Logger.Warningf("[%s] invalid api timeout, using default", d.Service)
		} else {
			d.APITimeout = time.Duration(apiTimeout)
		}
	}
	if d.ReadySignal != nil {
		close(d.ReadySignal)
	}
}

func (d *Driver) start(msg *Message) {
	if d.ReadySignal != nil {
		<-d.ReadySignal
		d.ReadySignal = nil
	}

	if d.State == "alive" {
		if d.Reload != nil {
			d.Reload(msg)
		} else {
			d.State = "cold"
		}
	} else {
		if d.Start != nil {
			d.Start(msg)
		}
		d.State = "alive"
	}
	d.Ping(msg)
}

func (d *Driver) stop(msg *Message) {
	d.State = "stopped"
	if d.Stop != nil {
		d.Stop(msg)
	}
	d.Ping(msg)
	Logger.Warningf("[%s] quiting on daemon request", d.Service)
}

// SendMessage : send a prepared message to daemon.
func (d *Driver) SendMessage(m *Message) {
	Logger.Infof("[%s] sending raw message to daemon %s:%s", d.Service, m.Channel, m.Subject)
	SendMessage(d.Out, m)
}

// CheckOption : get the data from options and check if it is truthy.
func (d *Driver) CheckOption(path string) bool {
	return CheckMapData(d.Options, path)
}

// GetOption : get the data from options map with the key.
func (d *Driver) GetOption(path string) (interface{}, bool) {
	return GetMapData(d.Options, path)
}

// GetOptionStr : get the string data from options map with the key.
func (d *Driver) GetOptionStr(path string) (string, bool) {
	return GetMapDataStr(d.Options, path)
}

// we have to keep hold of the os.File object to
// avoid being closed by garbage collector (runtime.setFinalizer).
var logFile *os.File

// GetLogger : getting a logger for the driver.
func (d *Driver) GetLogger() *logging.Logger {
	if Logger == nil {
		levelstr, ok := d.GetOptionStr("data.loglevel")
		if !ok {
			levelstr = "INFO"
		}
		if logFile == nil {
			logFile = os.NewFile(DriverLogDescriptor, "log")
		}

		return GetLogger(d.Name, levelstr, logFile)
	}

	return Logger
}

// GetReceiver getting a object that receive objects through SendMessage.
func (d *Driver) GetReceiver(feature string) interface{} {
	return d
}

// GetName returns the name of the driver.
func (d *Driver) GetName() string {
	return d.Name
}

// EmitEvent creates a new event.
func (d *Driver) EmitEvent(payload map[string]interface{}) string {
	id, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	d.SendMessage(&Message{
		Channel: "eventbus",
		Subject: "message",
		Payload: payload,
		Labels: map[string]string{
			"eventID": id.String(),
		},
	})

	return id.String()
}

// GetContext creates a context with APITimeout.
func (d *Driver) GetContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d.APITimeout*time.Second)
}

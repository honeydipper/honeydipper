package dipper

import (
	"github.com/op/go-logging"
	"io"
	"os"
	"time"
)

// RPCHandler : a type of functions that handle RPC calls between drivers
type RPCHandler func(string, string, []byte)

// Driver : the helper stuct for creating a honey-dipper driver in golang
type Driver struct {
	RPC struct {
		Caller   RPCCaller
		Provider RPCProvider
	}
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
	CommandProvider CommandProvider
}

// NewDriver : create a blank driver object
func NewDriver(service string, name string) *Driver {
	driver := Driver{
		Name:    name,
		Service: service,
		State:   "loaded",
		In:      os.Stdin,
		Out:     os.Stdout,
	}

	driver.RPC.Provider.Init("rpc", "return", driver.Out)
	driver.RPC.Caller.Init("rpc", "call")
	driver.CommandProvider.Init("eventbus", "return", driver.Out)

	driver.MessageHandlers = map[string]MessageHandler{
		"command:options":  driver.ReceiveOptions,
		"command:ping":     driver.Ping,
		"command:start":    driver.start,
		"command:stop":     driver.stop,
		"rpc:call":         driver.RPC.Provider.Router,
		"rpc:return":       driver.RPC.Caller.HandleReturn,
		"eventbus:command": driver.CommandProvider.Router,
	}

	driver.GetLogger()
	return &driver
}

// Run : start a loop to communicate with daemon
func (d *Driver) Run() {
	log.Infof("[%s] driver loaded", d.Service)
	for {
		func() {
			defer SafeExitOnError("[%s] Resuming driver message loop", d.Service)
			defer CatchError(io.EOF, func() {
				log.Fatalf("[%s] daemon closed channel", d.Service)
			})
			for {
				msg := FetchRawMessage(d.In)
				go func() {
					defer SafeExitOnError("[%s] Continuing driver message loop", d.Service)
					if handler, ok := d.MessageHandlers[msg.Channel+":"+msg.Subject]; ok {
						handler(msg)
					} else {
						log.Infof("[%s] skipping message without handler: %s:%s", d.Service, msg.Channel, msg.Subject)
					}
				}()
			}
		}()
	}
}

// Ping : respond to daemon ping request with driver state
func (d *Driver) Ping(msg *Message) {
	d.SendMessage(&Message{
		Channel: "state",
		Subject: d.State,
	})
}

// ReceiveOptions : receive options from daemon
func (d *Driver) ReceiveOptions(msg *Message) {
	msg = DeserializePayload(msg)
	Recursive(msg.Payload, RegexParser)
	d.Options = msg.Payload
	log = nil
	d.GetLogger()
	d.ReadySignal <- true
}

func (d *Driver) start(msg *Message) {
	select {
	case <-d.ReadySignal:
	case <-time.After(time.Second):
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
	d.State = "exit"
	if d.Stop != nil {
		d.Stop(msg)
	}
	d.Ping(msg)
	log.Fatalf("[%s] quiting on daemon request", d.Service)
}

// SendMessage : send a prepared message to daemon
func (d *Driver) SendMessage(m *Message) {
	log.Infof("[%s] sending raw message to daemon %s:%s", d.Service, m.Channel, m.Subject)
	SendMessage(d.Out, m)
}

// GetOption : get the data from options map with the key
func (d *Driver) GetOption(path string) (interface{}, bool) {
	return GetMapData(d.Options, path)
}

// GetOptionStr : get the string data from options map with the key
func (d *Driver) GetOptionStr(path string) (string, bool) {
	return GetMapDataStr(d.Options, path)
}

// RPCCallRaw : making a PRC call with raw bytes from driver to another driver
func (d *Driver) RPCCallRaw(feature string, method string, params []byte) ([]byte, error) {
	return d.RPC.Caller.CallRaw(d.Out, feature, method, params)
}

// RPCCall : making a PRC call from driver to another driver
func (d *Driver) RPCCall(feature string, method string, params interface{}) ([]byte, error) {
	return d.RPC.Caller.Call(d.Out, feature, method, params)
}

// GetLogger : getting a logger for the driver
func (d *Driver) GetLogger() *logging.Logger {
	if log == nil {
		levelstr, ok := d.GetOptionStr("data.loglevel")
		if !ok {
			levelstr = "INFO"
		}
		logFile := os.NewFile(uintptr(3), "log")
		return GetLogger(d.Name, levelstr, logFile)
	}
	return log
}

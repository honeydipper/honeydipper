package dipper

import (
	"io"
	"log"
	"os"
)

// Driver : the helper stuct for creating a honey-dipper driver in golang
type Driver struct {
	Name            string
	Service         string
	State           string
	In              io.Reader
	Out             io.Writer
	Options         interface{}
	MessageHandlers map[string]func(*Message)
	Start           func(*Message)
	Stop            func(*Message)
	Reload          func(*Message)
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

	driver.MessageHandlers = map[string]func(*Message){
		"command:options": driver.ReceiveOptions,
		"command:ping":    driver.Ping,
		"command:start":   driver.start,
		"command:stop":    driver.stop,
	}

	return &driver
}

// Run : start a loop to communicate with daemon
func (d *Driver) Run() {
	log.Printf("[%s-%s] driver loaded\n", d.Service, d.Name)
	func() {
		defer SafeExitOnError("[%s-%s] Resuming driver message loop", d.Service, d.Name)
		defer CatchError(io.EOF, func() {
			log.Fatalf("[%s-%s] daemon closed channel", d.Service, d.Name)
		})
		for {
			msg := FetchMessage(d.In)

			if handler, ok := d.MessageHandlers[msg.Channel+":"+msg.Subject]; ok {
				handler(msg)
			} else {
				log.Printf("[%s-%s] skipping message without handler: %s:%s", d.Service, d.Name, msg.Channel, msg.Subject)
			}
		}
	}()
}

// Ping : respond to daemon ping request with driver state
func (d *Driver) Ping(msg *Message) {
	d.SendMessage("state", d.State, nil)
}

// ReceiveOptions : receive options from daemon
func (d *Driver) ReceiveOptions(msg *Message) {
	if msg.IsRaw {
		msg = DeserializePayload(msg)
	}
	d.Options = msg.Payload
}

func (d *Driver) start(msg *Message) {
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
	d.State = "quiting"
	if d.Stop != nil {
		d.Stop(msg)
	}
	d.Ping(msg)
	log.Fatalf("[%s-%s] quiting on daemon request", d.Service, d.Name)
}

// SendRawMessage : construct and send a message to daemon
func (d *Driver) SendRawMessage(channel string, subject string, payload []byte) {
	log.Printf("[%s-%s] sending raw message to daemon %s:%s", d.Service, d.Name, channel, subject)
	SendRawMessage(d.Out, channel, subject, payload)
}

// SendMessage : send a prepared message to daemon
func (d *Driver) SendMessage(channel string, subject string, payload interface{}) {
	log.Printf("[%s-%s] sending raw message to daemon %s:%s", d.Service, d.Name, channel, subject)
	SendMessage(d.Out, channel, subject, payload)
}

// GetOption : get the data from options map with the key
func (d *Driver) GetOption(path string) (interface{}, bool) {
	return GetMapData(d.Options, path)
}

// GetOptionStr : get the string data from options map with the key
func (d *Driver) GetOptionStr(path string) (string, bool) {
	return GetMapDataStr(d.Options, path)
}

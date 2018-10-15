package dipper

import (
	"bufio"
	"io"
	"log"
	"os"
	"strings"
)

// Driver : the helper stuct for creating a honey-dipper driver in golang
type Driver struct {
	Name            string
	Service         string
	State           string
	In              *bufio.Reader
	Out             io.Writer
	Options         map[string]string
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
		In:      bufio.NewReader(os.Stdin),
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
			log.Fatalf("[%s-%s] daemon exited", d.Service, d.Name)
		})
		for {
			msg := FetchMessage(d.In)

			if handler, ok := d.MessageHandlers[msg.Channel+":"+msg.Subject]; ok {
				handler(&msg)
			} else {
				log.Printf("[%s-%s] skipping message without handler: %s:%s", d.Service, d.Name, msg.Channel, msg.Subject)
			}
		}
	}()
}

// Ping : respond to daemon ping request with driver state
func (d *Driver) Ping(msg *Message) {
	d.SendRawMessage("state", d.State, "", nil)
}

// ReceiveOptions : receive options from daemon
func (d *Driver) ReceiveOptions(msg *Message) {
	for _, line := range msg.Payload {
		parts := strings.Split(line, "=")
		key := parts[0]
		val := strings.Join(parts[1:], "=")
		if d.Options == nil {
			d.Options = make(map[string]string)
		}
		log.Printf("[%s-%s] receiving option %s", d.Service, d.Name, key)
		d.Options[key] = val
	}
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
	if d.Stop != nil {
		d.Stop(msg)
	}
	d.State = "left"
	d.Ping(msg)
}

// SendRawMessage : construct and send a message to daemon
func (d *Driver) SendRawMessage(channel string, subject string, payloadType string, payload []string) {
	log.Printf("[%s-%s] sending raw message to daemon %s:%s", d.Service, d.Name, channel, subject)
	SendRawMessage(d.Out, channel, subject, payloadType, payload)
}

// SendMessage : send a prepared message to daemon
func (d *Driver) SendMessage(msg *Message) {
	log.Printf("[%s-%s] sending raw message to daemon %s:%s", d.Service, d.Name, msg.Channel, msg.Subject)
	SendMessage(d.Out, msg)
}

// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

// Package dipper is a library used for developing drivers for Honeydipper.
package dipper

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// channel names and subject names.
const (
	ChannelEventbus = "eventbus"
	ChannelState    = "state"
	ChannelRPC      = "rpc"
	EventbusMessage = "message"
	EventbusCommand = "command"
	EventbusReturn  = "return"
)

// CommLocks : comm channels are protected with locks.
var CommLocks = map[io.Writer]*sync.Mutex{}

// MasterCommLock : the lock used to protect the comm locks.
var MasterCommLock = sync.Mutex{}

// Message : the message passed between components of the system.
type Message struct {
	// on the wire
	Channel string
	Subject string
	Size    int
	Labels  map[string]string
	Payload interface{}

	// runtime meta info in memory
	IsRaw    bool
	Reply    chan Message
	ReturnTo io.Writer
}

// MessageHandler : a type of functions that take a pointer to a message and handle it.
type MessageHandler func(*Message)

// SerializeContent : encode payload content into bytes.
func SerializeContent(content interface{}) (ret []byte) {
	var err error
	if content != nil {
		ret, err = json.Marshal(content)
		if err != nil {
			panic(err)
		}

		return ret
	}

	return []byte{}
}

// SerializePayload : encode a message payload return the modified message.
func SerializePayload(msg *Message) *Message {
	if !msg.IsRaw {
		if msg.Payload != nil {
			msg.Payload = SerializeContent(msg.Payload.([]byte))
		}

		msg.IsRaw = true
	}

	return msg
}

// DeserializeContent : decode the content into interface.
func DeserializeContent(content []byte) (ret interface{}) {
	ret = map[string]interface{}{}

	if len(content) > 0 {
		if err := json.Unmarshal(content, &ret); err != nil {
			panic(err)
		}

		return ret
	}

	return nil
}

// DeserializePayload : decode a message payload from bytes.
func DeserializePayload(msg *Message) *Message {
	if msg.IsRaw {
		if msg.Payload != nil {
			msg.Payload = DeserializeContent(msg.Payload.([]byte))
		}

		msg.IsRaw = false
	}

	return msg
}

// FetchMessage : fetch message from input from daemon service.
//   may block or throw io.EOF based on the fcntl setting.
func FetchMessage(in io.Reader) (msg *Message) {
	return DeserializePayload(FetchRawMessage(in))
}

// FetchRawMessage : fetch encoded message from input from daemon service.
//   may block or throw io.EOF based on the fcntl setting.
func FetchRawMessage(in io.Reader) (msg *Message) {
	var (
		channel   string
		subject   string
		size      int
		numLabels int
	)

	_, err := fmt.Fscanln(in, &channel, &subject, &numLabels, &size)
	if err == io.EOF {
		panic(err)
	} else if err != nil {
		errMsg := fmt.Sprintf("%+v", err)
		if strings.Contains(errMsg, "file already closed") {
			panic(io.EOF)
		}
		panic(fmt.Errorf("invalid message envelope: %w", err))
	}

	msg = &Message{
		Channel: channel,
		Subject: subject,
		IsRaw:   true,
		Size:    size,
	}

	msg.Labels = map[string]string{}
	for ; numLabels > 0; numLabels-- {
		var (
			lname string
			vl    int
		)

		_, err := fmt.Fscanln(in, &lname, &vl)
		if err != nil {
			panic(fmt.Errorf("unable to fetch message label name: %w", err))
		}
		if vl > 0 {
			lvalue := make([]byte, vl)
			if _, err = io.ReadFull(in, lvalue); err != nil {
				panic(fmt.Errorf("unable to fetch value for label %s: %w", lname, err))
			}

			msg.Labels[lname] = string(lvalue)
		} else {
			msg.Labels[lname] = ""
		}
	}

	if size > 0 {
		buf := make([]byte, size)
		_, err := io.ReadFull(in, buf)
		if err != nil {
			panic(err)
		}
		msg.Payload = buf
	}

	return msg
}

// SendMessage : send a message to the io.Writer, may change the message to raw.
func SendMessage(out io.Writer, msg *Message) {
	payload := []byte{}
	if msg.Payload != nil {
		if !msg.IsRaw {
			payload = SerializeContent(msg.Payload)
		} else {
			payload = msg.Payload.([]byte)
		}
	}
	size := len(payload)
	numLabels := len(msg.Labels)

	LockComm(out)
	defer UnlockComm(out)

	fmt.Fprintf(out, "%s %s %d %d\n", msg.Channel, msg.Subject, numLabels, size)
	if numLabels > 0 {
		for lname, lval := range msg.Labels {
			fmt.Fprintf(out, "%s %d\n", lname, len(lval))
			_, err := out.Write([]byte(lval))
			if err != nil {
				panic(err)
			}
		}
	}
	if size > 0 {
		_, err := out.Write(payload)
		if err != nil {
			panic(err)
		}
	}
}

// LockComm : Lock the comm channel.
func LockComm(out io.Writer) {
	var lock *sync.Mutex
	func() {
		MasterCommLock.Lock()
		defer MasterCommLock.Unlock()
		var ok bool
		lock, ok = CommLocks[out]
		if !ok {
			lock = &sync.Mutex{}
			CommLocks[out] = lock
		}
	}()
	lock.Lock()
}

// UnlockComm : unlock the comm channel.
func UnlockComm(out io.Writer) {
	var lock *sync.Mutex
	func() {
		MasterCommLock.Lock()
		defer MasterCommLock.Unlock()
		var ok bool
		lock, ok = CommLocks[out]
		if !ok {
			panic("comm lock not found")
		}
	}()
	lock.Unlock()
}

// RemoveComm : remove the lock when the comm channel is closed.
func RemoveComm(out io.Writer) {
	MasterCommLock.Lock()
	defer MasterCommLock.Unlock()
	delete(CommLocks, out)
}

// MessageCopy : performs a deep copy of the given map m.
func MessageCopy(m *Message) (*Message, error) {
	var buf bytes.Buffer
	if m == nil {
		return nil, nil
	}
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	err := enc.Encode(*m)
	if err != nil {
		return nil, err
	}
	var mcopy Message
	err = dec.Decode(&mcopy)
	if err != nil {
		return nil, err
	}

	return &mcopy, nil
}

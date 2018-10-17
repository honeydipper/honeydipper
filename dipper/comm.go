package dipper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
)

// CommLocks : comm channels are protected with locks
var CommLocks = map[io.Writer]*sync.Mutex{}

// MasterCommLock : the lock used to protect the comm locks
var MasterCommLock = sync.Mutex{}

// Message : the message passed between components of the system
type Message struct {
	Channel string
	Subject string
	Size    int
	IsRaw   bool
	Payload interface{}
}

// SerializePayload : encode a message payload into bytes
func SerializePayload(payload interface{}) (ret []byte) {
	var err error
	if payload != nil {
		ret, err = json.Marshal(payload)
		if err != nil {
			panic(err)
		}
		return ret
	}
	return []byte{}
}

// DeserializePayload : decode a message payload from bytes
func DeserializePayload(msg *Message) *Message {
	ret := map[string]interface{}{}
	if msg.Payload != nil && msg.IsRaw {
		if err := json.Unmarshal(msg.Payload.([]byte), &ret); err != nil {
			panic(err)
		}
		msg.Payload = ret
	}
	msg.IsRaw = false
	return msg
}

// FetchMessage : fetch message from input from daemon service
//   may block or throw io.EOF based on the fcntl setting
func FetchMessage(in io.Reader) (msg *Message) {
	return DeserializePayload(FetchRawMessage(in))
}

// FetchRawMessage : fetch encoded message from input from daemon service
//   may block or throw io.EOF based on the fcntl setting
func FetchRawMessage(in io.Reader) (msg *Message) {
	var channel string
	var subject string
	var size int

	_, err := fmt.Fscanln(in, &channel, &subject, &size)
	if err != nil {
		log.Panicf("invalid message envelope: %v", err)
	}

	msg = &Message{
		Channel: channel,
		Subject: subject,
		IsRaw:   true,
		Size:    size,
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

// SendMessage : send a message back to the daemon service
func SendMessage(out io.Writer, channel string, subject string, payload interface{}) {
	SendRawMessage(out, channel, subject, SerializePayload(payload))
}

// SendRawMessage : send unpackaged message back to the daemon service
func SendRawMessage(out io.Writer, channel string, subject string, payload []byte) {
	LockComm(out)
	defer UnlockComm(out)
	size := len(payload)
	fmt.Fprintf(out, "%s %s %d\n", channel, subject, size)
	if size > 0 {
		_, err := out.Write(payload)
		if err != nil {
			panic(err)
		}
	}
}

// LockComm : Lock the comm channel
func LockComm(out io.Writer) {
	MasterCommLock.Lock()
	defer MasterCommLock.Unlock()
	lock, ok := CommLocks[out]
	if !ok {
		lock = &sync.Mutex{}
		CommLocks[out] = lock
	}
	lock.Lock()
}

// UnlockComm : unlock the comm channel
func UnlockComm(out io.Writer) {
	MasterCommLock.Lock()
	defer MasterCommLock.Unlock()
	lock, ok := CommLocks[out]
	if !ok {
		panic("comm lock not found")
	}
	lock.Unlock()
}

// RemoveComm : remove the lock when the comm channel is closed
func RemoveComm(out io.Writer) {
	MasterCommLock.Lock()
	defer MasterCommLock.Unlock()
	if _, ok := CommLocks[out]; ok {
		delete(CommLocks, out)
	}
}

package dipper

import (
	"bufio"
	"fmt"
	"io"
	"sync"
)

// CommLocks : comm channels are protected with locks
var CommLocks = map[io.Writer]*sync.Mutex{}

// MasterCommLock : the lock used to protect the comm locks
var MasterCommLock = sync.Mutex{}

// Message : the message passed between components of the system
type Message struct {
	Channel     string
	Subject     string
	PayloadType string
	Payload     []string
}

// ReadField : read a field from the input use sep as separater
func ReadField(in *bufio.Reader, sep byte) string {
	val, err := in.ReadString(sep)
	if err != nil {
		panic(err)
	}
	return val[:len(val)-1]
}

// FetchMessage : fetch message from input from daemon service
//   may block or throw io.EOF based on the fcntl setting
func FetchMessage(in *bufio.Reader) (msg Message) {
	msg = Message{
		Channel:     ReadField(in, ':'),
		Subject:     ReadField(in, ':'),
		PayloadType: ReadField(in, '\n'),
	}
	if len(msg.PayloadType) > 0 {
		line := "init payload"
		for {
			line = ReadField(in, '\n')
			if len(line) > 0 {
				msg.Payload = append(msg.Payload, line)
			} else {
				break
			}
		}
	}

	return
}

// SendMessage : send a message back to the daemon service
func SendMessage(out io.Writer, msg *Message) {
	SendRawMessage(out, msg.Channel, msg.Subject, msg.PayloadType, msg.Payload)
}

// SendRawMessage : send unpackaged message back to the daemon service
func SendRawMessage(out io.Writer, channel string, subject string, payloadType string, payload []string) {
	LockComm(out)
	defer UnlockComm(out)
	fmt.Fprintf(out, "%s:%s:%s\n", channel, subject, payloadType)
	if len(payload) > 0 {
		for _, line := range payload {
			fmt.Fprintln(out, line)
		}
		fmt.Fprintln(out, "")
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

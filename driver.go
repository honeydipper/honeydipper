package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/honeyscience/honeydipper/dipper"
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"
)

// NewDriver : create a driver object to run the program process
func NewDriver(data map[string]interface{}) Driver {
	cmd, ok := data["Executable"].(string)
	if !ok {
		cmd = ""
	}

	args, ok := data["Arguments"].([]string)
	if !ok {
		args = []string{}
	}

	driver := Driver{
		Executable: cmd,
		Arguments:  args,
	}
	return driver
}

func (runtime *DriverRuntime) start(service string) {
	runtime.service = service

	if runtime.driver.PreStart != nil {
		runtime.driver.PreStart(service, runtime)
	}

	args := append([]string{service}, runtime.driver.Arguments...)
	run := exec.Command(runtime.driver.Executable, args...)
	if input, err := run.StdoutPipe(); err != nil {
		log.Panicf("Unable to link to driver stdout %v", err)
	} else {
		runtime.input = int(input.(*os.File).Fd())
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(runtime.input), syscall.F_GETFL, 0)
		if errno != 0 {
			panic(errno.Error())
		}
		flags |= syscall.O_NONBLOCK
		_, _, errno = syscall.Syscall(syscall.SYS_FCNTL, uintptr(runtime.input), syscall.F_SETFL, uintptr(flags))
		if errno != 0 {
			panic(errno.Error())
		}
	}
	if output, err := run.StdinPipe(); err != nil {
		log.Panicf("Unable to link to driver stdin %v", err)
	} else {
		runtime.output = &(output)
	}
	run.Stderr = os.Stderr
	if err := run.Start(); err != nil {
		log.Panicf("Failed to start driver %v", err)
	}

	msg := &dipper.Message{
		Channel:     "command",
		Subject:     "options",
		PayloadType: "kv",
	}

	dipper.ForEachRecursive("", *runtime.data, func(key string, val string) {
		log.Printf("sending to driver option:%s=%s\n", key, val)
		msg.Payload = append(msg.Payload, key+"="+val)
	})

	if len(msg.Payload) > 0 {
		runtime.sendMessage(msg)
	}

	startInst := &dipper.Message{
		Channel:     "command",
		Subject:     "start",
		PayloadType: "",
	}

	runtime.sendMessage(startInst)
}

func (runtime *DriverRuntime) sendMessage(msg *dipper.Message) {
	fmt.Fprintf(*runtime.output, "%s:%s:%s\n", msg.Channel, msg.Subject, msg.PayloadType)
	if len(msg.PayloadType) > 0 {
		for _, line := range msg.Payload {
			fmt.Fprintln(*runtime.output, line)
		}
		fmt.Fprintln(*runtime.output, "")
	}
}

func (runtime *DriverRuntime) fetchMessages() (messages []*dipper.Message) {
	defer dipper.SafeExitOnError(
		"failed to fetching messages from driver %s.%s",
		runtime.service,
		runtime.meta.Name,
	)
	var buf []byte
	var err error
	landing := make([]byte, 256)

	for l := 0; err == nil; {
		l, err = syscall.Read(runtime.input, landing)
		if l > 0 {
			buf = append(buf, landing[:l]...)
		}
		if err != nil && err != syscall.EAGAIN {
			panic(err)
		}
	}

	rd := bufio.NewReader(bytes.NewReader(buf))
	func() {
		defer dipper.IgnoreError(io.EOF)
		for {
			message := dipper.FetchMessage(rd)
			messages = append(messages, &message)
		}
	}()

	return
}

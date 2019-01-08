package main

import (
	"github.com/honeyscience/honeydipper/dipper"
	"io"
	"os"
	"os/exec"
)

// replace the func variable with mock during testing
var execCommand = exec.Command

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
	run := execCommand(runtime.driver.Executable, args...)
	if input, err := run.StdoutPipe(); err != nil {
		log.Panicf("[%s] Unable to link to driver stdout %v", service, err)
	} else {
		runtime.input = input
		runtime.stream = make(chan dipper.Message, 10)
		go runtime.fetchMessages()
	}
	if output, err := run.StdinPipe(); err != nil {
		log.Panicf("[%s] Unable to link to driver stdin %v", service, err)
	} else {
		runtime.output = output
	}
	run.Stderr = os.Stderr
	run.ExtraFiles = []*os.File{os.Stdout} // giving child process stdout for logging
	if err := run.Start(); err != nil {
		log.Panicf("[%s] Failed to start driver %v", service, err)
	}

	runtime.Run = run
	runtime.sendOptions()
}

func (runtime *DriverRuntime) sendOptions() {
	runtime.sendMessage(&dipper.Message{
		Channel: "command",
		Subject: "options",
		IsRaw:   false,
		Payload: map[string]interface{}{
			"data":        runtime.data,
			"dynamicData": runtime.dynamicData,
		},
	})
	runtime.sendMessage(&dipper.Message{
		Channel: "command",
		Subject: "start",
	})
}

func (runtime *DriverRuntime) sendMessage(msg *dipper.Message) {
	if runtime.feature != "emitter" {
		s := Services[runtime.service]
		if emitter, ok := s.driverRuntimes["emitter"]; ok && emitter.state == "alive" {
			s.counterIncr("honeydipper.local.message", []string{
				"service:" + runtime.service,
				"driver:" + runtime.meta.Name,
				"direction:outbound",
				"channel:" + msg.Channel,
				"subject:" + msg.Subject,
			})
		}
	}
	dipper.SendMessage(runtime.output, msg)
}

func (runtime *DriverRuntime) fetchMessages() {
	quit := false
	daemonChildren.Add(1)
	defer daemonChildren.Done()
	for !quit && !shuttingDown {
		func() {
			defer dipper.SafeExitOnError(
				"failed to fetching messages from driver %s.%s",
				runtime.service,
				runtime.meta.Name,
			)
			defer dipper.CatchError(io.EOF, func() { quit = true })
			for !quit && !shuttingDown {
				message := dipper.FetchRawMessage(runtime.input)
				runtime.stream <- *message
			}
		}()
	}
	log.Warningf("[%s-%s] driver close for business", runtime.service, runtime.meta.Name)
}

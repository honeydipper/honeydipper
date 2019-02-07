package driver

import (
	"github.com/honeyscience/honeydipper/internal/config"
	"github.com/honeyscience/honeydipper/internal/daemon"
	"github.com/honeyscience/honeydipper/pkg/dipper"
	"io"
	"os"
	"os/exec"
)

// replace the func variable with mock during testing
var execCommand = exec.Command

// NewDriver creates a driver object to represent a child process.
func NewDriver(data map[string]interface{}) config.Driver {
	cmd, ok := data["Executable"].(string)
	if !ok {
		cmd = ""
	}

	args, ok := data["Arguments"].([]string)
	if !ok {
		args = []string{}
	}

	drv := config.Driver{
		Executable: cmd,
		Arguments:  args,
	}
	return drv
}

// Runtime contains the runtime information of the running driver.
type Runtime struct {
	Meta        *config.DriverMeta
	Data        interface{}
	DynamicData interface{}
	Feature     string
	Stream      chan dipper.Message
	Input       io.ReadCloser
	Output      io.WriteCloser
	Service     string
	Run         *exec.Cmd
	State       int
	Handler     Handler
}

// Start the driver child process.  The "service" indicates which service this driver belongs to.
func (runtime *Runtime) Start(service string) {
	runtime.Service = service

	runtime.Handler.PreStart(service, runtime)

	args := append([]string{service}, runtime.Handler.Driver().Arguments...)
	run := execCommand(runtime.Handler.Driver().Executable, args...)
	if input, err := run.StdoutPipe(); err != nil {
		dipper.Logger.Panicf("[%s] Unable to link to driver stdout %v", service, err)
	} else {
		runtime.Input = input
		runtime.Stream = make(chan dipper.Message, 10)
		go runtime.fetchMessages()
	}
	if output, err := run.StdinPipe(); err != nil {
		dipper.Logger.Panicf("[%s] Unable to link to driver stdin %v", service, err)
	} else {
		runtime.Output = output
	}
	run.Stderr = os.Stderr
	run.ExtraFiles = []*os.File{os.Stdout} // giving child process stdout for logging
	if err := run.Start(); err != nil {
		dipper.Logger.Panicf("[%s] Failed to start driver %v", service, err)
	}

	runtime.Run = run
	runtime.SendOptions()
}

// SendOptions sends driver options and data to the child process as a dipper message.
func (runtime *Runtime) SendOptions() {
	runtime.SendMessage(&dipper.Message{
		Channel: "command",
		Subject: "options",
		IsRaw:   false,
		Payload: map[string]interface{}{
			"data":        runtime.Data,
			"dynamicData": runtime.DynamicData,
		},
	})
	runtime.SendMessage(&dipper.Message{
		Channel: "command",
		Subject: "start",
	})
}

// SendMessage sentds a dipper message to the driver child process.
func (runtime *Runtime) SendMessage(msg *dipper.Message) {
	if runtime.Feature != "emitter" {
		if emitter, ok := daemon.Emitters[runtime.Service]; ok {
			emitter.CounterIncr("honey.honeydipper.local.message", []string{
				"service:" + runtime.Service,
				"driver:" + runtime.Meta.Name,
				"direction:outbound",
				"channel:" + msg.Channel,
				"subject:" + msg.Subject,
			})
		}
	}
	dipper.SendMessage(runtime.Output, msg)
}

func (runtime *Runtime) fetchMessages() {
	quit := false
	daemon.Children.Add(1)
	defer daemon.Children.Done()
	for !quit && !daemon.ShuttingDown {
		func() {
			defer dipper.SafeExitOnError(
				"failed to fetching messages from driver %s.%s",
				runtime.Service,
				runtime.Meta.Name,
			)
			defer dipper.CatchError(io.EOF, func() { quit = true })
			for !quit && !daemon.ShuttingDown {
				message := dipper.FetchRawMessage(runtime.Input)
				runtime.Stream <- *message
			}
		}()
	}
	dipper.Logger.Warningf("[%s-%s] driver close for business", runtime.Service, runtime.Meta.Name)
}

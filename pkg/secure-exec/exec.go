package secureexec

import (
	"encoding/base64"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

type SecureExec struct {
	*dipper.Driver
}

// _syscallExec can be changed to a mock for testing.
var _syscallExce = syscall.Exec

// NewDriver create a blank driver object.
func NewDriver(service string, name string, opts ...dipper.DriverOption) *SecureExec {
	return &SecureExec{
		Driver: dipper.NewDriver(service, name, opts...),
	}
}

func (s *SecureExec) Run() {
	if s.Service != "exec" {
		s.Driver.Run()

		return
	}

	method := "lookup"
	fn, ok := s.RPCHandlers["lookup"]
	if !ok {
		fn = s.RPCHandlers["decrypt"]
		method = "decrypt"
	}

	prefix := "hd-" + method + ":"
	l := len(prefix)

	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)

		if len(parts) > 1 && strings.HasPrefix(parts[1], prefix) {
			var value []byte
			if method == "decrypt" {
				value = dipper.Must(base64.StdEncoding.DecodeString(parts[1][l:])).([]byte)
			} else {
				value = []byte(parts[1][l:])
			}

			msg := &dipper.Message{
				Payload: value,
				IsRaw:   true,
				Reply:   make(chan dipper.Message),
			}
			go fn(msg)

			resp := <-msg.Reply
			os.Setenv(parts[0], string(resp.Payload.([]byte)))
		}
	}

	pos := slices.Index(os.Args, "--")
	if pos < 1 {
		pos = 1
	}

	args := []string{}
	if len(os.Args) > pos+2 {
		args = os.Args[pos+1:]
	}

	dipper.Must(_syscallExce(os.Args[pos+1], args, os.Environ()))
}

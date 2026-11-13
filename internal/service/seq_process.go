package service

import (
	"fmt"

	"github.com/honeydipper/honeydipper/v4/internal/driver"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
)

// process routes an inbound message either to a per-sequence goroutine for
// ordered, synchronous handling or fires it off asynchronously when no
// sequence label is present.
func (s *Service) process(msg dipper.Message, runtime *driver.Runtime) {
	seq := msg.Labels["sequence"]
	if seq == "" {
		go s.asyncProcess(msg, runtime)

		return
	}

	s.sequenceLock.Lock()
	defer s.sequenceLock.Unlock()

	seqChan, ok := s.sequences[seq]
	if !ok {
		// First message for this sequence key: create the channel and start the
		// dedicated goroutine that will drain it in order.
		seqChan = make(chan dipper.Message, 100)
		s.sequences[seq] = seqChan
		go s.sequenceProcess(seq, seqChan, runtime)
	}

	seqChan <- msg
}

// sequenceProcess drains seqChan in FIFO order, processing each message
// synchronously.  When the channel is empty the goroutine closes it, removes
// the entry from s.sequences, and exits — allowing WindDown to detect that all
// in-flight sequence work has finished.
func (s *Service) sequenceProcess(seq string, seqChan chan dipper.Message, runtime *driver.Runtime) {
	for {
		s.sequenceLock.Lock()
		select {
		case m := <-seqChan:
			s.sequenceLock.Unlock()
			s.syncProcess(&m, runtime)
		default:
			// Channel is empty; clean up and exit.
			close(seqChan)
			delete(s.sequences, seq)
			s.sequenceLock.Unlock()

			return
		}
	}
}

// asyncProcess handles a message concurrently: expect handlers, responders, and
// the transformer/router pipeline each run in their own goroutines.
func (s *Service) asyncProcess(msg dipper.Message, runtime *driver.Runtime) {
	defer dipper.SafeExitOnError("[%s] continue async processing loop", s.name)

	expectKey := fmt.Sprintf("%s:%s", msg.Channel, msg.Subject)
	if runtime != nil {
		expectKey = fmt.Sprintf("%s:%s:%s", msg.Channel, msg.Subject, runtime.Handler.Meta().Name)
	}

	if expects, ok := s.deleteExpect(expectKey); ok {
		for _, f := range expects {
			go func(f ExpectHandler) {
				defer dipper.SafeExitOnError("[%s] continue async processing loop", s.name)
				f(&msg)
			}(f)
		}
	}

	key := fmt.Sprintf("%s:%s", msg.Channel, msg.Subject)
	// responder
	if responders, ok := s.responders[key]; ok {
		for _, f := range responders {
			go func(f MessageResponder) {
				defer dipper.SafeExitOnError("[%s] continue async processing loop", s.name)
				f(runtime, &msg)
			}(f)
		}
	}

	go func(msg *dipper.Message) {
		defer dipper.SafeExitOnError("[%s] continue async processing loop", s.name)

		// transformer
		if transformers, ok := s.transformers[key]; ok {
			for _, f := range transformers {
				msg = f(runtime, msg)
				if msg == nil {
					break
				}
			}
		}

		if msg != nil && s.Route != nil {
			// router
			routedMsgs := s.Route(msg)

			if len(routedMsgs) > 0 {
				for _, routedMsg := range routedMsgs {
					routedMsg.driverRuntime.SendMessage(routedMsg.message)
				}
			}
		}
	}(&msg)
}

// syncProcess handles a message synchronously in the caller's goroutine:
// expect handlers, responders, transformers, and the router are all invoked in
// sequence with no additional goroutine spawning.
func (s *Service) syncProcess(msg *dipper.Message, runtime *driver.Runtime) {
	defer dipper.SafeExitOnError("[%s] continue sync processing loop", s.name)

	expectKey := fmt.Sprintf("%s:%s", msg.Channel, msg.Subject)
	if runtime != nil {
		expectKey = fmt.Sprintf("%s:%s:%s", msg.Channel, msg.Subject, runtime.Handler.Meta().Name)
	}

	if expects, ok := s.deleteExpect(expectKey); ok {
		for _, f := range expects {
			f(msg)
		}
	}

	key := fmt.Sprintf("%s:%s", msg.Channel, msg.Subject)
	// responder
	if responders, ok := s.responders[key]; ok {
		for _, f := range responders {
			f(runtime, msg)
		}
	}

	// transformer
	if transformers, ok := s.transformers[key]; ok {
		for _, f := range transformers {
			msg = f(runtime, msg)
			if msg == nil {
				break
			}
		}
	}

	if msg != nil && s.Route != nil {
		// router
		routedMsgs := s.Route(msg)

		if len(routedMsgs) > 0 {
			for _, routedMsg := range routedMsgs {
				routedMsg.driverRuntime.SendMessage(routedMsg.message)
			}
		}
	}
}

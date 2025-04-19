package main

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/go-redis/redismock/v8"
	"github.com/honeydipper/honeydipper/drivers/pkg/redisclient"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if dipper.Logger == nil {
		f, _ := os.Create("test.log")
		defer f.Close()
		dipper.GetLogger("test service", "DEBUG", f, f)
	}
	os.Exit(m.Run())
}

func testStartDriver(t *testing.T, service string, i *io.PipeReader, o *io.PipeWriter) <-chan struct{} {
	driver = dipper.NewDriver(service, "redisqueue", dipper.DriverWithReader(i), dipper.DriverWithWriter(o))
	done := make(chan struct{})
	go func() {
		assert.NotPanics(t, main, "driver main should not panic")
		close(done)
		driver = nil
	}()

	return done
}

func TestLoadDriver(t *testing.T) {
	i, inbuf := io.Pipe()
	outbuf, o := io.Pipe()
	done := testStartDriver(t, "test-service", i, o)

	dipper.SendMessage(inbuf, &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"connection": map[string]interface{}{
					"Addr":     "1.1.1.1:6379",
					"Username": "nouser",
					"Password": "123",
					"DB":       "2",
				},
			},
		},
	})
	dipper.SendMessage(inbuf, &dipper.Message{
		Channel: "command",
		Subject: "start",
	})

	reply := dipper.FetchRawMessage(outbuf)
	assert.Equal(t, "state", reply.Channel, "reply channel should be state")
	assert.Equal(t, "alive", reply.Subject, "reply subject should be alive")
	_, exists := dipper.GetMapData(driver.Options, "data.connection.Password")
	assert.False(t, exists, "Password should be removed from the driver options")
	assert.NotNil(t, redisOptions, "redisOptions should not be nil afterwards")

	driver.State = dipper.DriverStateCompleted
	inbuf.Close()
	<-done
}

func TestOperatorRelayToRedis(t *testing.T) {
	db, mock := redismock.NewClientMock()
	redisOptionsMock = &redisclient.Options{
		Client: db,
	}

	i, inbuf := io.Pipe()
	outbuf, o := io.Pipe()

	done := testStartDriver(t, "operator", i, o)
	dipper.SendMessage(inbuf, &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"connection": map[string]interface{}{
					"Addr":     "1.1.1.1:6379",
					"Username": "nouser",
					"Password": "123",
					"DB":       "2",
				},
			},
		},
	})
	dipper.SendMessage(inbuf, &dipper.Message{
		Channel: "command",
		Subject: "start",
	})

	reply := dipper.FetchRawMessage(outbuf)
	assert.Equal(t, "state", reply.Channel, "reply channel should be state")
	assert.Equal(t, "alive", reply.Subject, "reply subject should be alive")

	driver.State = dipper.DriverStateCompleted

	mock.MatchExpectationsInOrder(false)
	mock.ExpectBLPop(time.Second, "honeydipper:commands").SetVal([]string{})
	mock.ExpectRPush("honeydipper:events", `{"data":"{\"foo\":\"bar\"}","labels":{"from":"`+dipper.GetIP()+`"}}`).SetVal(1)

	dipper.SendMessage(inbuf, &dipper.Message{
		Channel: "eventbus",
		Subject: "message",
		Payload: map[string]interface{}{
			"foo": "bar",
		},
	})
	time.Sleep(time.Millisecond * 100)
	assert.NoError(t, mock.ExpectationsWereMet(), "mock redis expectations not met")
	inbuf.Close()
	<-done
}

func TestOperatorEmit(t *testing.T) {
	db, mock := redismock.NewClientMock()
	redisOptionsMock = &redisclient.Options{
		Client: db,
	}

	i, inbuf := io.Pipe()
	outbuf, o := io.Pipe()

	done := testStartDriver(t, "operator", i, o)
	dipper.SendMessage(inbuf, &dipper.Message{
		Channel: "command",
		Subject: "options",
		Payload: map[string]interface{}{
			"data": map[string]interface{}{
				"connection": map[string]interface{}{
					"Addr":     "1.1.1.1:6379",
					"Username": "nouser",
					"Password": "123",
					"DB":       "2",
				},
			},
		},
	})
	dipper.SendMessage(inbuf, &dipper.Message{
		Channel: "command",
		Subject: "start",
	})

	reply := dipper.FetchRawMessage(outbuf)
	assert.Equal(t, "state", reply.Channel, "reply channel should be state")
	assert.Equal(t, "alive", reply.Subject, "reply subject should be alive")
	driver.State = "completed"

	mock.MatchExpectationsInOrder(false)
	mock.ExpectBLPop(time.Second, "honeydipper:commands").SetVal([]string{"honeydipper:command", `{"labels": {"from": "1.1.1.1"}, "data": {"foo": "bar"}}`})

	reply = dipper.FetchRawMessage(outbuf)
	assert.Equal(t, "eventbus", reply.Channel, "reply channel should be state")
	assert.Equal(t, "command", reply.Subject, "reply subject should be alive")

	time.Sleep(time.Millisecond * 100)
	assert.NoError(t, mock.ExpectationsWereMet(), "mock redis expectations not met")

	inbuf.Close()
	<-done
}

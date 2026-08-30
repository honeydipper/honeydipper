// Copyright 2026 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

package secureexec

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"github.com/stretchr/testify/assert"
)

func TestNewDriver(t *testing.T) {
	t.Run("creates driver with correct service and name", func(t *testing.T) {
		driver := NewDriver("test-service", "test-name")

		assert.NotNil(t, driver)
		assert.Equal(t, "test-service", driver.Service)
		assert.Equal(t, "test-name", driver.Name)
		assert.NotNil(t, driver.Driver)
	})

	t.Run("passes options to underlying driver", func(t *testing.T) {
		called := false
		option := func(d *dipper.Driver) {
			called = true
		}

		driver := NewDriver("test-service", "test-name", option)

		assert.True(t, called, "option function should be called")
		assert.NotNil(t, driver.Driver)
	})
}

func TestSecureExec_Run(t *testing.T) {
	// Save original syscall.Exec for restoration
	originalSyscallExec := _syscallExce
	defer func() { _syscallExce = originalSyscallExec }()

	// Save original os.Args for restoration
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Save original environment for restoration
	originalEnviron := os.Environ()
	defer func() {
		// Restore environment
		os.Clearenv()
		for _, env := range originalEnviron {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	t.Run("exec service with lookup handler processes environment variables", func(t *testing.T) {
		execCalled := false
		var execPath string
		var execArgs []string
		var execEnv []string

		_syscallExce = func(path string, argv []string, envv []string) error {
			execCalled = true
			execPath = path
			execArgs = argv
			execEnv = envv

			return nil
		}

		// Clear environment and set up test environment
		os.Clearenv()
		os.Setenv("PATH", "/usr/bin")
		os.Setenv("TEST_VAR", "hd-lookup:secret-value")
		os.Setenv("NORMAL_VAR", "normal-value")

		// Set up args
		os.Args = []string{"test", "exec", "/bin/echo", "hello"}

		driver := NewDriver("exec", "test-name")

		// Set up lookup handler
		lookupCalled := false
		var receivedPayload []byte
		driver.RPCHandlers = map[string]dipper.MessageHandler{
			"lookup": func(msg *dipper.Message) {
				lookupCalled = true
				receivedPayload = msg.Payload.([]byte)
				// Send response
				msg.Reply <- dipper.Message{Payload: []byte("decrypted-secret")}
			},
		}

		driver.Run()

		assert.True(t, execCalled, "syscall.Exec should be called")
		assert.True(t, lookupCalled, "lookup handler should be called")
		assert.Equal(t, []byte("secret-value"), receivedPayload, "handler should receive correct payload")
		assert.Equal(t, "/bin/echo", execPath, "exec path should be correct")
		assert.Equal(t, []string{"/bin/echo", "hello"}, execArgs, "exec args should be correct")

		// Check that environment variable was updated
		found := false
		for _, env := range execEnv {
			if strings.HasPrefix(env, "TEST_VAR=") {
				assert.Equal(t, "TEST_VAR=decrypted-secret", env, "environment variable should be updated with decrypted value")
				found = true

				break
			}
		}
		assert.True(t, found, "TEST_VAR should be in environment with updated value")

		// Check that normal variable is unchanged
		found = false
		for _, env := range execEnv {
			if strings.HasPrefix(env, "NORMAL_VAR=") {
				assert.Equal(t, "NORMAL_VAR=normal-value", env, "normal environment variable should be unchanged")
				found = true

				break
			}
		}
		assert.True(t, found, "NORMAL_VAR should be in environment unchanged")
	})

	t.Run("exec service with decrypt handler processes base64 encoded values", func(t *testing.T) {
		execCalled := false
		_syscallExce = func(path string, argv []string, envv []string) error {
			execCalled = true

			return nil
		}

		// Clear environment and set up test environment
		os.Clearenv()
		os.Setenv("PATH", "/usr/bin")
		encodedValue := base64.StdEncoding.EncodeToString([]byte("secret-data"))
		os.Setenv("ENCRYPTED_VAR", "hd-decrypt:"+encodedValue)

		// Set up args
		os.Args = []string{"test", "exec", "/bin/echo", "hello"}

		driver := NewDriver("exec", "test-name")

		// Set up decrypt handler
		decryptCalled := false
		var receivedPayload []byte
		driver.RPCHandlers = map[string]dipper.MessageHandler{
			"decrypt": func(msg *dipper.Message) {
				decryptCalled = true
				receivedPayload = msg.Payload.([]byte)
				// Send response
				msg.Reply <- dipper.Message{Payload: []byte("decrypted-data")}
			},
		}

		driver.Run()

		assert.True(t, execCalled, "syscall.Exec should be called")
		assert.True(t, decryptCalled, "decrypt handler should be called")
		assert.Equal(t, []byte("secret-data"), receivedPayload, "handler should receive decoded payload")
	})

	t.Run("exec service prefers lookup over decrypt when both available", func(t *testing.T) {
		execCalled := false
		_syscallExce = func(path string, argv []string, envv []string) error {
			execCalled = true

			return nil
		}

		// Clear environment and set up test environment
		os.Clearenv()
		os.Setenv("PATH", "/usr/bin")
		os.Setenv("TEST_VAR", "hd-lookup:some-value")

		// Set up args
		os.Args = []string{"test", "/bin/echo", "hello"}

		driver := NewDriver("exec", "test-name")

		// Set up both handlers
		lookupCalled := false
		decryptCalled := false
		driver.RPCHandlers = map[string]dipper.MessageHandler{
			"lookup": func(msg *dipper.Message) {
				lookupCalled = true
				msg.Reply <- dipper.Message{Payload: []byte("lookup-result")}
			},
			"decrypt": func(msg *dipper.Message) {
				decryptCalled = true
				msg.Reply <- dipper.Message{Payload: []byte("decrypt-result")}
			},
		}

		driver.Run()

		assert.True(t, execCalled, "syscall.Exec should be called")
		assert.True(t, lookupCalled, "lookup handler should be called")
		assert.False(t, decryptCalled, "decrypt handler should not be called when lookup is available")
	})

	t.Run("exec service uses decrypt when lookup not available", func(t *testing.T) {
		execCalled := false
		_syscallExce = func(path string, argv []string, envv []string) error {
			execCalled = true

			return nil
		}

		// Clear environment and set up test environment
		os.Clearenv()
		os.Setenv("PATH", "/usr/bin")
		os.Setenv("TEST_VAR", "hd-decrypt:"+base64.StdEncoding.EncodeToString([]byte("secret-value")))

		// Set up args
		os.Args = []string{"test", "/bin/echo", "hello"}

		driver := NewDriver("exec", "test-name")

		// Set up only decrypt handler
		decryptCalled := false
		driver.RPCHandlers = map[string]dipper.MessageHandler{
			"decrypt": func(msg *dipper.Message) {
				decryptCalled = true
				assert.Equal(t, []byte("secret-value"), msg.Payload.([]byte))
				msg.Reply <- dipper.Message{Payload: []byte("decrypt-result")}
			},
		}

		driver.Run()

		assert.True(t, execCalled, "syscall.Exec should be called")
		assert.True(t, decryptCalled, "decrypt handler should be called when lookup not available")
	})

	t.Run("ignores environment variables without correct prefix", func(t *testing.T) {
		execCalled := false
		_syscallExce = func(path string, argv []string, envv []string) error {
			execCalled = true

			return nil
		}

		// Clear environment and set up test environment
		os.Clearenv()
		os.Setenv("PATH", "/usr/bin")
		os.Setenv("WRONG_PREFIX", "wrong-prefix:some-value")
		os.Setenv("NO_PREFIX", "some-value")

		// Set up args
		os.Args = []string{"test", "/bin/echo", "hello"}

		driver := NewDriver("exec", "test-name")

		// Set up lookup handler
		lookupCalled := false
		driver.RPCHandlers = map[string]dipper.MessageHandler{
			"lookup": func(msg *dipper.Message) {
				lookupCalled = true
				msg.Reply <- dipper.Message{Payload: []byte("should-not-be-called")}
			},
		}

		driver.Run()

		assert.True(t, execCalled, "syscall.Exec should be called")
		assert.False(t, lookupCalled, "lookup handler should not be called for wrong prefixes")
	})

	t.Run("handles multiple environment variables with prefixes", func(t *testing.T) {
		execCalled := false
		_syscallExce = func(path string, argv []string, envv []string) error {
			execCalled = true

			return nil
		}

		// Clear environment and set up test environment
		os.Clearenv()
		os.Setenv("PATH", "/usr/bin")
		os.Setenv("VAR1", "hd-lookup:value1")
		os.Setenv("VAR2", "hd-lookup:value2")
		os.Setenv("VAR3", "normal-var")

		// Set up args
		os.Args = []string{"test", "/bin/echo", "hello"}

		driver := NewDriver("exec", "test-name")

		callCount := 0
		driver.RPCHandlers = map[string]dipper.MessageHandler{
			"lookup": func(msg *dipper.Message) {
				callCount++
				payload := string(msg.Payload.([]byte))
				if payload == "value1" {
					msg.Reply <- dipper.Message{Payload: []byte("result1")}
				} else if payload == "value2" {
					msg.Reply <- dipper.Message{Payload: []byte("result2")}
				}
			},
		}

		driver.Run()

		assert.True(t, execCalled, "syscall.Exec should be called")
		assert.Equal(t, 2, callCount, "lookup handler should be called twice")

		// Check that both variables were updated
		var1Found := false
		var2Found := false
		var3Found := false
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "VAR1=") {
				assert.Equal(t, "VAR1=result1", env)
				var1Found = true
			} else if strings.HasPrefix(env, "VAR2=") {
				assert.Equal(t, "VAR2=result2", env)
				var2Found = true
			} else if strings.HasPrefix(env, "VAR3=") {
				assert.Equal(t, "VAR3=normal-var", env)
				var3Found = true
			}
		}
		assert.True(t, var1Found, "VAR1 should be updated")
		assert.True(t, var2Found, "VAR2 should be updated")
		assert.True(t, var3Found, "VAR3 should remain unchanged")
	})

	t.Run("handles invalid base64 in decrypt mode", func(t *testing.T) {
		// This test verifies that invalid base64 causes a panic (as expected by dipper.Must)
		os.Clearenv()
		os.Setenv("PATH", "/usr/bin")
		os.Setenv("INVALID_VAR", "hd-decrypt:invalid-base64!")

		os.Args = []string{"test", "/bin/echo", "hello"}

		driver := NewDriver("exec", "test-name")
		driver.RPCHandlers = map[string]dipper.MessageHandler{
			"decrypt": func(msg *dipper.Message) {
				msg.Reply <- dipper.Message{Payload: []byte("result")}
			},
		}

		assert.Panics(t, func() { driver.Run() }, "should panic on invalid base64")
	})

	t.Run("handles environment variables with equals signs", func(t *testing.T) {
		execCalled := false
		_syscallExce = func(path string, argv []string, envv []string) error {
			execCalled = true

			return nil
		}

		// Clear environment and set up test environment
		os.Clearenv()
		os.Setenv("PATH", "/usr/bin")
		os.Setenv("COMPLEX_VAR", "hd-lookup:value=with=equals")

		// Set up args
		os.Args = []string{"test", "/bin/echo", "hello"}

		driver := NewDriver("exec", "test-name")

		lookupCalled := false
		driver.RPCHandlers = map[string]dipper.MessageHandler{
			"lookup": func(msg *dipper.Message) {
				lookupCalled = true
				assert.Equal(t, []byte("value=with=equals"), msg.Payload.([]byte))
				msg.Reply <- dipper.Message{Payload: []byte("processed=value")}
			},
		}

		driver.Run()

		assert.True(t, execCalled, "syscall.Exec should be called")
		assert.True(t, lookupCalled, "lookup handler should be called")

		// Check that environment variable was updated correctly
		found := false
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "COMPLEX_VAR=") {
				assert.Equal(t, "COMPLEX_VAR=processed=value", env)
				found = true

				break
			}
		}
		assert.True(t, found, "COMPLEX_VAR should be updated correctly")
	})
}

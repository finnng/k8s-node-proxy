package helpers

import (
	"context"
	"os"
	"testing"
)

// TestEnv manages test environment setup and teardown
type TestEnv struct {
	t               *testing.T
	originalEnvVars map[string]string
}

// NewTestEnv creates a new test environment manager
func NewTestEnv(t *testing.T) *TestEnv {
	return &TestEnv{
		t:               t,
		originalEnvVars: make(map[string]string),
	}
}

// SetEnv sets an environment variable and remembers the original value
func (e *TestEnv) SetEnv(key, value string) {
	if _, exists := e.originalEnvVars[key]; !exists {
		e.originalEnvVars[key] = os.Getenv(key)
	}
	os.Setenv(key, value)
}

// Cleanup restores original environment variables
func (e *TestEnv) Cleanup() {
	for key, originalValue := range e.originalEnvVars {
		if originalValue == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, originalValue)
		}
	}
}

// Context returns a test context with timeout
func (e *TestEnv) Context() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	e.t.Cleanup(cancel)
	return ctx
}

const testTimeout = 30 // seconds

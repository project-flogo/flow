package support

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetConcurrentExecution covers every branch of the env-flag helper: unset (default),
// truthy, falsey, and an unparseable value (which falls back to the default).
func TestGetConcurrentExecution(t *testing.T) {
	orig, had := os.LookupEnv(ConcurrentExecution)
	defer func() {
		if had {
			_ = os.Setenv(ConcurrentExecution, orig)
		} else {
			_ = os.Unsetenv(ConcurrentExecution)
		}
	}()

	_ = os.Unsetenv(ConcurrentExecution)
	assert.False(t, GetConcurrentExecution(), "unset must default to false")

	_ = os.Setenv(ConcurrentExecution, "true")
	assert.True(t, GetConcurrentExecution())

	_ = os.Setenv(ConcurrentExecution, "1")
	assert.True(t, GetConcurrentExecution(), "coerce should accept 1 as true")

	_ = os.Setenv(ConcurrentExecution, "false")
	assert.False(t, GetConcurrentExecution())

	_ = os.Setenv(ConcurrentExecution, "not-a-bool")
	assert.False(t, GetConcurrentExecution(), "unparseable value must default to false")
}

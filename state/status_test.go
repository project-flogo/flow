package state

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestToRecordingMode(t *testing.T) {
	m, err := ToRecordingMode("OFF")
	assert.Nil(t, err)
	assert.Equal(t, StateModeOff, m)
	m, err = ToRecordingMode("Debugger")
	assert.Nil(t, err)
	assert.Equal(t, StateModeDebugger, m)
	m, err = ToRecordingMode("dddddd")
	assert.NotNil(t, err)
}

package model

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRegistry(t *testing.T) {
	flowModel := &FlowModel{name :"test"}

	Register(flowModel)

	assert.Equal(t, 1, len(Registered()))

	fM, err := Get("test")
	assert.Nil(t, err)
	assert.NotNil(t, fM)
}

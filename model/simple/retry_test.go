package simple

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRetryData(t *testing.T) {
	taskTestContext := &TestTaskContext{}

	rData , err := getRetryData(taskTestContext)
	assert.Nil(t, err)
	assert.NotNil(t, rData)
}

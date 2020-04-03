package simple

import (
	"github.com/project-flogo/flow/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDoWhileTaskBehavior(t *testing.T){

	testContext := &TestTaskContext{}

	doWhileTaskBehaviour := &DoWhileTaskBehavior{}

	assert.NotNil(t,doWhileTaskBehaviour.Enter(testContext))
	result , err := doWhileTaskBehaviour.Eval(testContext)
	assert.Nil(t, err)
	assert.Equal(t, model.EvalDone, result)
	notify, tasks, skip := doWhileTaskBehaviour.Skip(testContext)
	assert.True(t, notify)
	assert.Nil(t, tasks)
	assert.True(t, skip)
}

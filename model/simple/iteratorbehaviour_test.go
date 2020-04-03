package simple

import (
	"github.com/project-flogo/flow/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIteratorTaskBehavior(t *testing.T){

	testContext := &TestTaskContext{}

	iteratorTaskBehaviour := &IteratorTaskBehavior{}

	assert.NotNil(t,iteratorTaskBehaviour.Enter(testContext))
	result , err := iteratorTaskBehaviour.Eval(testContext)
	assert.Nil(t, err)
	assert.Equal(t, model.EvalDone, result)
	notify, tasks, skip := iteratorTaskBehaviour.Skip(testContext)
	assert.True(t, notify)
	assert.Nil(t, tasks)
	assert.True(t, skip)
}

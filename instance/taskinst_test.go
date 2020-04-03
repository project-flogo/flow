package instance

import (
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTaskInst(t *testing.T) {

	def := getDef()
	ind , _ := NewIndependentInstance("test","", def, log.RootLogger())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	assert.NotNil(t, taskInst)
	assert.True(t, taskInst.HasActivity())

	result, err := taskInst.EvalLink(def.Links()[0])
	assert.Nil(t, err)
	assert.True(t, result)

	//done, err := taskInst.EvalActivity()
	//assert.True(t, done)
	//assert.Nil(t, err)

	assert.Nil(t,taskInst.FlowLogger())
	assert.Nil(t, taskInst.GetTracingContext())
	taskInst.SetStatus(model.TaskStatusDone)
	assert.Equal(t, model.TaskStatusDone, taskInst.Status())
	assert.Nil(t, taskInst.GetFromLinkInstances())
}


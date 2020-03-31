package instance

import (
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/support/log"
	"github.com/stretchr/testify/assert"
	"testing"
)

func init() {
	_ = activity.LegacyRegister("log", NewLogActivity())
}

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

	done, err := taskInst.EvalActivity()
	assert.True(t, done)
	assert.Nil(t, err)

	assert.Nil(t,taskInst.FlowLogger())
}


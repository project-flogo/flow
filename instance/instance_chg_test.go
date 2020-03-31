package instance

import (
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/model"
	"github.com/stretchr/testify/assert"
	"testing"
)


func TestSimpleChange(t *testing.T) {
	def := getDef()
	//fmt.Println(def.ModelID())

	ind , err := NewIndependentInstance("test","", def, log.RootLogger())
	assert.Nil(t, err)
	assert.NotNil(t, ind)

	chgTrackingEnabled = true
	chgTrack := NewInstanceChangeTracker("test")
	assert.NotNil(t, chgTrack)

	assert.NotNil(t,chgTrack.ExtractStep(false))
	chgTrack.SetStatus(1, model.FlowStatusActive)
	chgTrack.AttrChange(1, "key", "value")
	chgTrack.FlowCreated(ind)
	chgTrack.ExtractStep(true)
}
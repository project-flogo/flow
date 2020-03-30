package instance

import (
	"github.com/project-flogo/flow/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSimpleChange(t *testing.T) {
	chgTrackingEnabled = true

	chgTrack := NewInstanceChangeTracker("test")
	assert.NotNil(t, chgTrack)

	assert.NotNil(t,chgTrack.ExtractStep(false))
	chgTrack.SetStatus(1, model.FlowStatusActive)
	chgTrack.AttrChange(1, "key", "value")
}
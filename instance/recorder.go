package instance

import (
	"github.com/project-flogo/flow/state"
	"time"
)

type stateInstanceRecorder struct {
	mod              state.RecordingMode
	externalRecorder state.Recorder
}

func NewStateInstnaceRecorder(recorder state.Recorder, mod state.RecordingMode) *stateInstanceRecorder {
	return &stateInstanceRecorder{
		mod:              mod,
		externalRecorder: recorder,
	}
}

func (inst *IndependentInstance) RecordState(strtTime time.Time) error {
	if state.RecordSnapshot(inst.instRecorder.mod) {
		err := inst.instRecorder.externalRecorder.RecordSnapshot(inst.Snapshot())
		if err != nil {
			inst.logger.Warnf("unable to record snapshot: %v", err)
		}
	}

	if state.RecordSteps(inst.instRecorder.mod) {
		currStep := inst.CurrentStep(true)
		currStep.StartTime = strtTime
		currStep.EndTime = time.Now().UTC()
		err := inst.instRecorder.externalRecorder.RecordStep(currStep)
		if err != nil {
			inst.logger.Warnf("unable to record step: %v", err)
		}
	}
	return nil
}

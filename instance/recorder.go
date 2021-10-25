package instance

import (
	"github.com/project-flogo/flow/state"
	"time"
)

type stateInstanceRecorder struct {
	mod              state.RecordingMode
	externalRecorder state.Recorder
	rerun            bool
}

func NewStateInstanceRecorder(recorder state.Recorder, mod state.RecordingMode, rerunstate bool) *stateInstanceRecorder {
	return &stateInstanceRecorder{
		mod:              mod,
		externalRecorder: recorder,
		rerun: rerunstate,
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
		currStep.Rerun = inst.instRecorder.rerun
		err := inst.instRecorder.externalRecorder.RecordStep(currStep)
		if err != nil {
			inst.logger.Warnf("unable to record step: %v", err)
		}
	}
	return nil
}

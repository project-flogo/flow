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
		rerun:            rerunstate,
	}
}

func (inst *IndependentInstance) RecordState(strtTime time.Time) error {
	// Pre-existing, surfaced by FLOGO-19484: ondemand/action.go builds its instance with a nil
	// instRecorder, while handleGlobalError and handleCancelError call RecordState
	// unconditionally on the embedded-instance path - so an on-demand flow whose subflow errors
	// nil-derefs below. This ticket's rollback paths run straight through those call sites, so
	// the one-line guard is taken here rather than left as a trap.
	if inst.instRecorder == nil {
		return nil
	}

	// FLOGO-19484 / D10 (record side). Only stamp when something is actually being recorded, so
	// the marker is not written into $flow for apps that have recording off entirely.
	if state.RecordSnapshot(inst.instRecorder.mod) || state.RecordSteps(inst.instRecorder.mod) {
		inst.stampTxInFlight()
	}

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

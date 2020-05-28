package state

import (
	"fmt"
	"github.com/project-flogo/core/data/coerce"
	"strings"
)

type RecordingMode string

const (
	// RecordingModeOff indicates that the recording been turned off
	RecordingModeOff RecordingMode = "off"
	// RecordingModeDebugger incicates that the state recording store steps data as well as task's input
	RecordingModeDebugger RecordingMode = "debugger"
	// RecordingModeStep incicates that the state recording to store steps data only
	RecordingModeStep RecordingMode = "step"
	// RecordingModeFull incicates that the state recording to store both steps and snapshot data
	RecordingModeFull RecordingMode = "full"
	// RecordingModeSnapshot incicates that the state recording to store snapshot data only
	RecordingModeSnapshot RecordingMode = "snapshot"
)

// ToRecordingMode convert data to recording model const
func ToRecordingMode(mode interface{}) (RecordingMode, error) {
	m, _ := coerce.ToString(mode)
	rMode := RecordingMode(strings.ToLower(m))
	switch rMode {
	case RecordingModeDebugger, RecordingModeOff, RecordingModeFull, RecordingModeSnapshot, RecordingModeStep:
		return rMode, nil
	default:
		return RecordingModeOff, fmt.Errorf("unsupport state recording mode [%s]", m)
	}
}

// RecordSteps check to see if enbale step recording
func RecordSteps(stateRecordingMode RecordingMode) bool {
	switch stateRecordingMode {
	case RecordingModeOff, RecordingModeSnapshot:
		return false
	default:
		return true
	}
}

// RecordSnapshot check to see if enbale snapshot recording
func RecordSnapshot(stateRecordingMode RecordingMode) bool {
	switch stateRecordingMode {
	case RecordingModeSnapshot, RecordingModeFull:
		return true
	default:
		return false
	}
}

package state

// Recorder is the interface that describes a service that can record
// snapshots and steps of a Flow Instance
type Recorder interface {
	RecordStart(state *FlowState) error
	// RecordSnapshot records a Snapshot of the FlowInstance
	RecordSnapshot(snapshot *Snapshot) error

	// RecordStep records the changes for the current Step of the Flow Instance
	RecordStep(step *Step) error

	RecordDone(state *FlowState) error
}

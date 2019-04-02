package event

import (
	"time"
)

type Status string

const (
	CREATED   = "Created"
	COMPLETED = "Completed"
	CANCELLED = "Cancelled"
	FAILED    = "Failed"
	SCHEDULED = "Scheduled"
	SKIPPED   = "Skipped"
	STARTED   = "Started"
	WAITING   = "Waiting"
	UNKNOWN   = "Created"
)

const FlowEventType = "flowevent"
const TaskEventType = "taskevent"

// FlowEvent provides access to flow instance execution details
type FlowEvent interface {
	// Returns flow name
	FlowName() string
	// Returns flow ID
	FlowID() string
	// In case of subflow, returns parent flow name
	ParentFlowName() string
	// In case of subflow, returns parent flow ID
	ParentFlowID() string
	// Returns event time
	Time() time.Time
	// Returns current flow status
	FlowStatus() Status
	// Returns input data for flow instance
	FlowInput() map[string]interface{}
	// Returns output data for completed flow instance
	FlowOutput() map[string]interface{}
	// Returns error for failed flow instance
	FlowError() error
}

// TaskEvent provides access to task instance execution details
type TaskEvent interface {
	// Returns flow name
	FlowName() string
	// Returns flow ID
	FlowID() string
	// Returns task name
	TaskName() string
	// Returns task type
	TaskType() string
	// Returns task status
	TaskStatus() Status
	// Returns event time
	Time() time.Time
	// Returns task input data
	TaskInput() map[string]interface{}
	// Returns task output data for completed task
	TaskOutput() map[string]interface{}
	// Returns error for failed task
	TaskError() error
}

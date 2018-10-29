package instance

import (
	"github.com/project-flogo/flow/support"
)

const (
	OpStart   = iota // 0
	OpResume         // 1
	OpRestart        // 2
)

// RunOptions the options when running a FlowAction
type RunOptions struct {
	Op           int
	ReturnID     bool
	FlowURI      string
	InitialState *IndependentInstance
	ExecOptions  *ExecOptions
}

// ExecOptions are optional Patch & Interceptor to be used during instance execution
type ExecOptions struct {
	Patch       *support.Patch
	Interceptor *support.Interceptor
}

// IDGenerator generates IDs for flow instances
type IDGenerator interface {
	//NewFlowInstanceID generate a new instance ID
	NewFlowInstanceID() string
}

// ApplyExecOptions applies any execution options to the flow instance
func ApplyExecOptions(instance *IndependentInstance, execOptions *ExecOptions) {

	if execOptions != nil {

		if execOptions.Patch != nil {
			instance.logger.Debugf("Instance [%s] has patch", instance.ID())
			instance.patch = execOptions.Patch
			instance.patch.Init()
		}

		if execOptions.Interceptor != nil {
			instance.logger.Debugf("Instance [%s] has interceptor", instance.ID())
			instance.interceptor = execOptions.Interceptor
			instance.interceptor.Init()
		}
	}
}

// IDResponse is a response object consists of an ID
type IDResponse struct {
	ID string `json:"id"`
}

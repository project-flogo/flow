package support

// Interceptor contains a set of task interceptor, this can be used to override
// runtime data of an instance of the corresponding Flow.  This can be used to
// modify runtime execution of a flow or in test/debug for implementing mocks
// for tasks
type Interceptor struct {
	TaskInterceptors []*TaskInterceptor `json:"tasks"`

	taskInterceptorMap map[string]*TaskInterceptor
}

// Init initializes the FlowInterceptor, usually called after deserialization
func (pi *Interceptor) Init() {

	numAttrs := len(pi.TaskInterceptors)
	if numAttrs > 0 {

		pi.taskInterceptorMap = make(map[string]*TaskInterceptor, numAttrs)

		for _, interceptor := range pi.TaskInterceptors {
			pi.taskInterceptorMap[interceptor.ID] = interceptor
		}
	}
}

// GetTaskInterceptor get the TaskInterceptor for the specified task (referred to by ID)
func (pi *Interceptor) GetTaskInterceptor(taskID string) *TaskInterceptor {
	return pi.taskInterceptorMap[taskID]
}

// TaskInterceptor contains instance override information for a Task, such has attributes.
// Also, a 'Skip' flag can be enabled to inform the runtime that the task should not
// execute.
type TaskInterceptor struct {
	ID      string                 `json:"id"`
	Skip    bool                   `json:"skip,omitempty"`
	Inputs  map[string]interface{} `json:"inputs,omitempty"`
	Outputs map[string]interface{} `json:"outputs,omitempty"`
}

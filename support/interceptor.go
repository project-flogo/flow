package support

const (
	Primitive = 1
	Activity  = 2
	Link      = 3
)

// Interceptor contains a set of task interceptor, this can be used to override
// runtime data of an instance of the corresponding Flow.  This can be used to
// modify runtime execution of a flow or in test/debug for implementing mocks
// for tasks
type Interceptor struct {
	TaskInterceptors []*TaskInterceptor `json:"tasks"`

	taskInterceptorMap map[string]*TaskInterceptor

	Coverage *FlowCoverage
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
	pi.Coverage = &FlowCoverage{
		Links:      []string{},
		Activities: []string{},
	}
}

// GetTaskInterceptor get the TaskInterceptor for the specified task (referred to by ID)
func (pi *Interceptor) GetTaskInterceptor(taskID string) *TaskInterceptor {
	return pi.taskInterceptorMap[taskID]
}

func (pi *Interceptor) AddCoverage(task string, coverageType int) {
	switch coverageType {
	case Link:
		pi.Coverage.Links = append(pi.Coverage.Links, task)
	case Activity:
		pi.Coverage.Activities = append(pi.Coverage.Activities, task)
	}

}

// TaskInterceptor contains instance override information for a Task, such has attributes.
// Also, a 'Skip' flag can be enabled to inform the runtime that the task should not
// execute.
type TaskInterceptor struct {
	ID         string                 `json:"id"`
	Skip       bool                   `json:"skip,omitempty"`
	Inputs     map[string]interface{} `json:"inputs,omitempty"`
	Outputs    map[string]interface{} `json:"outputs,omitempty"`
	Assertions []Assertion            `json:"assertions,omitempty"`
}

type Assertion struct {
	ID         string
	Name       string
	Type       int
	Expression interface{}
	Result     bool
}

type FlowCoverage struct {
	Activities []string `json:"activities,omitempty"`
	Links      []string `json:"links,omitempty"`
}

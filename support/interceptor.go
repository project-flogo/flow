package support

import "github.com/project-flogo/core/data/expression/script/gocc/ast"

const (
	Primitive = 1
	Activity  = 2
)

const (
	AssertionActivity  = 1
	AssertionException = 2
	SkipActivity       = 3
	MockActivity       = 4
	MockException      = 5
)

const (
	NotExecuted          = 0
	Pass                 = 1
	Fail                 = 2
	Mocked               = 3
	AssertionNotExecuted = 4
)

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
	ID            string                 `json:"id"`
	Skip          bool                   `json:"skip,omitempty"`
	Inputs        map[string]interface{} `json:"inputs,omitempty"`
	Outputs       map[string]interface{} `json:"outputs,omitempty"`
	Assertions    []Assertion            `json:"assertions,omitempty"`
	SkipExecution bool                   `json:"skipExecution"`
	Result        int                    `json:"result,omitempty"`
	Message       string                 `json:"message"`
	Type          int                    `json:"type"`
}

type Assertion struct {
	ID         string
	Name       string
	Type       int
	Expression interface{}
	Result     int
	Message    string
	EvalResult ast.ExprEvalData
}

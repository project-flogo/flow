package model

import (
	"github.com/project-flogo/flow/definition"
)

// TaskEntry is a struct used to specify what Task to
// enter and its corresponding enter code
type TaskEntry struct {
	Task      *definition.Task
	EnterCode int
}

// FlowBehavior is the execution behavior of the Flow.
type FlowBehavior interface {

	// Start the flow instance.  Returning true indicates that the
	// flow can start and enter the specified tasks.
	// Return false indicates that the flow could not be started
	// at this time.
	Start(context FlowContext) (started bool, taskEntries []*TaskEntry)

	// StartErrorHandler start the error handler for the flow.
	// Return the list of tasks to start
	StartErrorHandler(context FlowContext) (taskEntries []*TaskEntry)

	// Resume the flow instance.  Returning true indicates that the
	// flow can resume.  Return false indicates that the flow
	// could not be resumed at this time.
	Resume(context FlowContext) (resumed bool)

	// TasksDone is called when a terminal task is Done.
	TaskDone(context FlowContext) (flowDone bool)

	// Done is called when the flow is done.
	Done(context FlowContext)
}

type EvalResult int

const (
	EvalFail EvalResult = iota
	EvalDone
	EvalRepeat
	EvalWait
	EvalSkip
)

type EnterResult int

const (
	ERNotReady EnterResult = iota
	EREval
	ERSkip
)

// TaskBehavior is the execution behavior of a Task.
type TaskBehavior interface {

	// Enter determines if a Task is ready to be evaluated, or needs to be
	// skipped
	Enter(context TaskContext) (enterResult EnterResult)

	// Eval is called when a Task is being evaluated.  Returning true indicates
	// that the task is done.  If err is set, it indicates that the
	// behavior intends for the flow ErrorHandler to handle the error
	Eval(context TaskContext) (evalResult EvalResult, err error)

	// PostEval is called when a task that is waiting needs to be notified.
	// If err is set, it indicates that the behavior intends for the
	// flow ErrorHandler to handle the error
	PostEval(context TaskContext) (evalResult EvalResult, err error)

	// Done is called when Eval or PostEval return a result of DONE, indicating
	// that the task is done.  This step is used to finalize the task and
	// determine the next set of tasks to be entered.
	Done(context TaskContext) (notifyFlow bool, taskEntries []*TaskEntry, err error)

	// Skip is called when Enter returns a result of SKIP, indicating
	// that the task should be skipped.  This step is used to skip the task and
	// determine the next set of tasks to be entered.
	Skip(context TaskContext) (notifyFlow bool, taskEntries []*TaskEntry, propagateSkip bool)

	// Error is called when there is an issue executing Eval, it returns a boolean indicating
	// if it handled the error, otherwise the error is handled by the global error handler
	Error(context TaskContext, err error) (handled bool, taskEntries []*TaskEntry)
}

package simple

import (
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/model"
)

// FlowBehavior implements model.FlowBehavior
type FlowBehavior struct {
}

// Start implements model.FlowBehavior.Start
func (fb *FlowBehavior) Start(ctx model.FlowContext) (started bool, taskEntries []*model.TaskEntry) {
	return true, getFlowTaskEntries(ctx.FlowDefinition().Tasks(), true)
}

// StartErrorHandler implements model.FlowBehavior.StartErrorHandler
func (fb *FlowBehavior) StartErrorHandler(ctx model.FlowContext) (taskEntries []*model.TaskEntry) {
	return getFlowTaskEntries(ctx.FlowDefinition().GetErrorHandler().Tasks(), true)
}

// Resume implements model.FlowBehavior.Resume
func (fb *FlowBehavior) Resume(ctx model.FlowContext) (resumed bool) {
	return true
}

// TasksDone implements model.FlowBehavior.TasksDone
func (fb *FlowBehavior) TaskDone(ctx model.FlowContext) (flowDone bool) {
	tasks := ctx.TaskInstances()

	logger := ctx.Logger()

	logger.Debug("Checking if all tasks done or skipped")

	for _, taskInst := range tasks {

		if taskInst.Status() < model.TaskStatusDone { //ignore not started?

			logger.Debugf("Task '%s' not done or skipped", taskInst.Task().ID())
			return false
		}
	}

	logger.Debug("All tasks done or skipped")

	// our tasks are done, so the flow is done
	return true
}

// Done implements model.FlowBehavior.Done
func (fb *FlowBehavior) Done(ctx model.FlowContext) {
	ctx.Logger().Debug("Flow Done")
}

func getFlowTaskEntries(tasks []*definition.Task, leadingOnly bool) []*model.TaskEntry {

	var taskEntries []*model.TaskEntry

	for _, task := range tasks {

		if len(task.FromLinks()) == 0 || !leadingOnly {

			taskEntry := &model.TaskEntry{Task: task, EnterCode: 0}
			taskEntries = append(taskEntries, taskEntry)
		}
	}

	return taskEntries
}

package simple

import (
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/support/trace"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
)

////////////////////////////////////////////////////////////////////////////////////////////////////////

// TaskBehavior implements model.TaskBehavior
type TaskBehavior struct {
}

// Enter implements model.TaskBehavior.Enter
func (tb *TaskBehavior) Enter(ctx model.TaskContext) (enterResult model.EnterResult) {

	logger := ctx.FlowLogger()

	task := ctx.Task()

	if logger.DebugEnabled() {
		logger.Debugf("Enter Task '%s'", task.ID())
	}

	ctx.SetStatus(model.TaskStatusEntered)

	//check if all predecessor links are done
	linkInsts := ctx.GetFromLinkInstances()

	ready := true
	skipped := false

	if len(linkInsts) == 0 {
		// has no predecessor links, so task is ready
		ready = true
	} else {
		skipped = true

		if logger.DebugEnabled() {
			logger.Debugf("Task '%s' has %d incoming Links", task.ID(), len(linkInsts))
		}
		for _, linkInst := range linkInsts {
			if logger.DebugEnabled() {
				logger.Debugf("Task '%s': Link from Task '%s' has status '%s'", task.ID(), linkInst.Link().FromTask().ID(), linkStatus(linkInst))
			}
			if linkInst.Status() < model.LinkStatusFalse {
				ready = false
				break
			} else if linkInst.Status() == model.LinkStatusTrue {
				skipped = false
			}
		}
	}

	if ready {

		if skipped {
			ctx.SetStatus(model.TaskStatusSkipped)
			return model.EnterSkip
		} else {
			if logger.DebugEnabled() {
				logger.Debugf("Task '%s' Ready", ctx.Task().ID())
			}
			ctx.SetStatus(model.TaskStatusReady)
		}
		return model.EnterEval

	} else {
		if logger.DebugEnabled() {
			logger.Debugf("Task '%s' Not Ready", ctx.Task().ID())
		}
	}

	return model.EnterNotReady
}

// Eval implements model.TaskBehavior.Eval
func (tb *TaskBehavior) Eval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {

	if ctx.Status() == model.TaskStatusSkipped {
		return model.EvalSkip, nil
	}

	task := ctx.Task()
	ctx.FlowLogger().Debugf("Eval Task '%s'", task.ID())

	done, err := evalActivity(ctx)
	if err != nil {
		ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
		ctx.FlowLogger().Errorf("Error evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
		ctx.SetStatus(model.TaskStatusFailed)
		return model.EvalFail, err
	}

	if done {
		evalResult = model.EvalDone
	} else {
		evalResult = model.EvalWait
	}

	return evalResult, nil
}

//todo this shouldn't be accessing instance

func evalActivity(ctx model.TaskContext) (bool, error) {
	done, err := ctx.EvalActivity()

	if err != nil {
		// check if error returned is retriable
		if errVal, ok := err.(*activity.Error); ok && errVal.Retriable() {

			// check if task is configured to retry on error
			retryData, rerr := getRetryData(ctx)
			if rerr != nil {
				return done, rerr
			}

			if t, ok := ctx.(*instance.TaskInst); ok {
				if t.GetTracingContext() != nil {
					t.GetTracingContext().SetTag("retry_enabled", true)
					t.GetTracingContext().SetTag("retries_remaining", retryData.Count)
					t.GetTracingContext().SetTag("retry_interval_ms", retryData.Interval)
					// Complete previous trace except last. Last trace will completed in the caller.
					if retryData.Count > 0 {
						_ = trace.GetTracer().FinishTrace(t.GetTracingContext(), err)
					}
				}
			}

			if retryData != nil && retryData.Count > 0 {
				return retryEval(ctx, retryData)
			}
		}
		return done, err
	}
	return done, nil
}

// PostEval implements model.TaskBehavior.PostEval
func (tb *TaskBehavior) PostEval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	ctx.FlowLogger().Debugf("PostEval Task '%s'", ctx.Task().ID())
	_, err = ctx.PostEvalActivity()
	if err != nil {
		//// check if error returned is retriable
		//if errVal, ok := err.(*activity.Error); ok && errVal.Retriable() {
		//	// check if task is configured to retry on error
		//	retryData, rerr := getRetryData(ctx)
		//	if rerr != nil {
		//		return model.EvalFail, rerr
		//	}
		//	if retryData.Count > 0 {
		//		return retryPostEval(ctx, retryData), nil
		//	}
		//}
		ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
		ctx.FlowLogger().Errorf("Error post evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
		ctx.SetStatus(model.TaskStatusFailed)
		return model.EvalFail, err
	}
	return model.EvalDone, nil
}

// Done implements model.TaskBehavior.Done
func (tb *TaskBehavior) Done(ctx model.TaskContext) (notifyFlow bool, taskEntries []*model.TaskEntry, err error) {

	logger := ctx.FlowLogger()

	linkInsts := ctx.GetToLinkInstances()
	numLinks := len(linkInsts)

	ctx.SetStatus(model.TaskStatusDone)

	if logger.DebugEnabled() {
		logger.Debugf("Task '%s' is done", ctx.Task().ID())
	}

	// process outgoing links
	if numLinks > 0 {

		taskEntries = make([]*model.TaskEntry, 0, numLinks)

		if logger.DebugEnabled() {
			logger.Debugf("Task '%s' has %d outgoing links", ctx.Task().ID(), numLinks)
		}

		var exprLinkFollowed, hasExprLink bool
		var exprOtherwiseLinkInst model.LinkInstance
		for _, linkInst := range linkInsts {

			follow := true

			if linkInst.Link().Type() == definition.LtError {
				//todo should we skip or ignore?
				continue
			}

			if linkInst.Link().Type() == definition.LtExprOtherwise {
				exprOtherwiseLinkInst = linkInst
				continue
			}

			if linkInst.Link().Type() == definition.LtExpression {
				hasExprLink = true
				//todo handle error
				if logger.DebugEnabled() {
					logger.Debugf("Task '%s': Evaluating Outgoing Expression Link to Task '%s'", ctx.Task().ID(), linkInst.Link().ToTask().ID())
				}
				follow, err = ctx.EvalLink(linkInst.Link())
				if err != nil {
					return false, nil, err
				}
				if follow {
					exprLinkFollowed = true
				}
			}

			if follow {
				linkInst.SetStatus(model.LinkStatusTrue)
				if logger.DebugEnabled() {
					logger.Debugf("Task '%s': Following Link  to task '%s'", ctx.Task().ID(), linkInst.Link().ToTask().ID())
				}
				taskEntry := &model.TaskEntry{Task: linkInst.Link().ToTask()}
				taskEntries = append(taskEntries, taskEntry)
			} else {
				linkInst.SetStatus(model.LinkStatusFalse)

				taskEntry := &model.TaskEntry{Task: linkInst.Link().ToTask()}
				taskEntries = append(taskEntries, taskEntry)
			}
		}

		//Otherwise branch while no link to follow
		if hasExprLink && !exprLinkFollowed && exprOtherwiseLinkInst != nil {
			exprOtherwiseLinkInst.SetStatus(model.LinkStatusTrue)
			if logger.DebugEnabled() {
				logger.Debugf("Task '%s': Following Link  to task '%s'", ctx.Task().ID(), exprOtherwiseLinkInst.Link().ToTask().ID())
			}
			taskEntry := &model.TaskEntry{Task: exprOtherwiseLinkInst.Link().ToTask()}
			taskEntries = append(taskEntries, taskEntry)
		}

		//continue on to successor tasks
		return false, taskEntries, nil
	}

	if logger.DebugEnabled() {
		logger.Debugf("Notifying flow that end task '%s' is done", ctx.Task().ID())
	}

	// there are no outgoing links, so just notify parent that we are done
	return true, nil, nil
}

// Skip implements model.TaskBehavior.Skip
func (tb *TaskBehavior) Skip(ctx model.TaskContext) (notifyFlow bool, taskEntries []*model.TaskEntry) {
	linkInsts := ctx.GetToLinkInstances()
	numLinks := len(linkInsts)

	ctx.SetStatus(model.TaskStatusSkipped)

	logger := ctx.FlowLogger()

	if logger.DebugEnabled() {
		logger.Debugf("Task '%s' was skipped", ctx.Task().ID())
	}

	// process outgoing links
	if numLinks > 0 {

		taskEntries = make([]*model.TaskEntry, 0, numLinks)

		if logger.DebugEnabled() {
			logger.Debugf("Task '%s' has %d outgoing links", ctx.Task().ID(), numLinks)
		}

		for _, linkInst := range linkInsts {
			linkInst.SetStatus(model.LinkStatusSkipped)
			taskEntry := &model.TaskEntry{Task: linkInst.Link().ToTask()}
			taskEntries = append(taskEntries, taskEntry)
		}

		return false, taskEntries
	}

	if logger.DebugEnabled() {
		logger.Debugf("Notifying flow that end task '%s' is skipped", ctx.Task().ID())
	}

	return true, nil
}

// Error implements model.TaskBehavior.Error
func (tb *TaskBehavior) Error(ctx model.TaskContext, err error) (handled bool, taskEntries []*model.TaskEntry) {

	linkInsts := ctx.GetToLinkInstances()
	numLinks := len(linkInsts)

	handled = false

	// process outgoing links
	if numLinks > 0 {

		for _, linkInst := range linkInsts {
			if linkInst.Link().Type() == definition.LtError {
				handled = true
				break
			}
		}

		if handled {
			taskEntries = make([]*model.TaskEntry, 0, numLinks)

			for _, linkInst := range linkInsts {

				if linkInst.Link().Type() == definition.LtError {
					linkInst.SetStatus(model.LinkStatusTrue)
				} else {
					linkInst.SetStatus(model.LinkStatusFalse)
				}

				taskEntry := &model.TaskEntry{Task: linkInst.Link().ToTask()}
				taskEntries = append(taskEntries, taskEntry)
			}

			return true, taskEntries
		}
	}

	return false, nil
}

func linkStatus(inst model.LinkInstance) string {

	switch inst.Status() {
	case model.LinkStatusFalse:
		return "false"
	case model.LinkStatusTrue:
		return "true"
	case model.LinkStatusSkipped:
		return "skipped"
	}

	return "unknown"
}

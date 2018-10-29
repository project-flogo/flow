package simple

import (
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/flow/definition"
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
			return model.ENTER_SKIP
		} else {
			if logger.DebugEnabled() {
				logger.Debugf("Task '%s' Ready", ctx.Task().ID())
			}
			ctx.SetStatus(model.TaskStatusReady)
		}
		return model.ENTER_EVAL

	} else {
		if logger.DebugEnabled() {
			logger.Debugf("Task '%s' Not Ready", ctx.Task().ID())
		}
	}

	return model.ENTER_NOTREADY
}

// Eval implements model.TaskBehavior.Eval
func (tb *TaskBehavior) Eval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {

	if ctx.Status() == model.TaskStatusSkipped {
		return model.EVAL_SKIP, nil
	}

	task := ctx.Task()
	ctx.FlowLogger().Debugf("Eval Task '%s'", task.ID())

	done, err := ctx.EvalActivity()

	if err != nil {
		ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
		ctx.FlowLogger().Errorf("Error evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
		ctx.SetStatus(model.TaskStatusFailed)
		return model.EVAL_FAIL, err
	}

	if done {
		evalResult = model.EVAL_DONE
	} else {
		evalResult = model.EVAL_WAIT
	}

	return evalResult, nil
}

// PostEval implements model.TaskBehavior.PostEval
func (tb *TaskBehavior) PostEval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {


	ctx.FlowLogger().Debugf("PostEval Task '%s'", ctx.Task().ID())

	_, err = ctx.PostEvalActivity()

	//what to do if eval isn't "done"?
	if err != nil {
		ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
		ctx.FlowLogger().Errorf("Error post evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
		ctx.SetStatus(model.TaskStatusFailed)
		return model.EVAL_FAIL, err
	}

	return model.EVAL_DONE, nil
}

// Done implements model.TaskBehavior.Done
func (tb *TaskBehavior) Done(ctx model.TaskContext) (notifyFlow bool, taskEntries []*model.TaskEntry, err error) {

	logger:= ctx.FlowLogger()

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

		for _, linkInst := range linkInsts {

			follow := true

			if linkInst.Link().Type() == definition.LtError {
				//todo should we skip or ignore?
				continue
			}

			if linkInst.Link().Type() == definition.LtExpression {
				//todo handle error
				if logger.DebugEnabled() {
					logger.Debugf("Task '%s': Evaluating Outgoing Expression Link to Task '%s'", ctx.Task().ID(), linkInst.Link().ToTask().ID())
				}
				follow, err = ctx.EvalLink(linkInst.Link())

				if err != nil {
					return false, nil, err
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

		//continue on to successor tasks
		return false, taskEntries, nil
	}

	if logger.DebugEnabled() {
		logger.Debugf("Notifying flow that end task '%s' is done", ctx.Task().ID())
	}

	// there are no outgoing links, so just notify parent that we are done
	return true, nil, nil
}

// Done implements model.TaskBehavior.Skip
func (tb *TaskBehavior) Skip(ctx model.TaskContext) (notifyFlow bool, taskEntries []*model.TaskEntry) {
	linkInsts := ctx.GetToLinkInstances()
	numLinks := len(linkInsts)

	ctx.SetStatus(model.TaskStatusSkipped)

	logger:= ctx.FlowLogger()

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

// Done implements model.TaskBehavior.Error
func (tb *TaskBehavior) Error(ctx model.TaskContext, err error) (handled bool, taskEntries []*model.TaskEntry) {

	linkInsts := ctx.GetToLinkInstances()
	numLinks := len(linkInsts)

	handled = false

	// process outgoing links
	if numLinks > 0 {

		for _, linkInst := range linkInsts {
			if linkInst.Link().Type() == definition.LtError {
				handled = true
			}
			break
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

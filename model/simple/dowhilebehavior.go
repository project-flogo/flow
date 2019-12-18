package simple

import (
	"time"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
)

// DoWhileTaskBehavior implements model.TaskBehavior
type DoWhileTaskBehavior struct {
	TaskBehavior
}

// DoWhile struct
type DoWhile struct {
	Index int `json:"index"`
}

// Eval implements model.TaskBehavior.Eval
func (dw *DoWhileTaskBehavior) Eval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	logger := ctx.FlowLogger()

	if ctx.Status() == model.TaskStatusSkipped {
		return model.EvalDone, nil //todo introduce EVAL_SKIP?
	}

	if logger.DebugEnabled() {
		logger.Debugf("Eval doWhile Task '%s'", ctx.Task().ID())
	}

	//Firs time, set index to 0
	initIndex(ctx)
	done, err := evalActivity(ctx)
	if err != nil {
		ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
		ctx.FlowLogger().Errorf("Error evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
		ctx.SetStatus(model.TaskStatusFailed)
		return model.EvalFail, err
	}

	if !done {
		ctx.SetStatus(model.TaskStatusWaiting)
		return model.EvalWait, nil
	}
	defer dw.updateDoWhileIndex(ctx)
	return dw.checkDoWhileCondition(ctx)
}

// PostEval implements model.TaskBehavior.PostEval
func (dw *DoWhileTaskBehavior) PostEval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	ctx.FlowLogger().Debugf("PostEval doWhile Task '%s'", ctx.Task().ID())

	initIndex(ctx)
	_, err = ctx.PostEvalActivity()

	if err != nil {
		ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
		ctx.FlowLogger().Errorf("Error post evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
		ctx.SetStatus(model.TaskStatusFailed)
		return model.EvalFail, err
	}
	ctx.SetStatus(model.TaskStatusDone)
	defer dw.updateDoWhileIndex(ctx)
	return dw.checkDoWhileCondition(ctx)
}

func (dw *DoWhileTaskBehavior) checkDoWhileCondition(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	if ctx.Task().LoopConfig() != nil && ctx.Task().LoopConfig().DowhileCondition() != nil {
		return dw.evaluateCondition(ctx, ctx.Task().LoopConfig().DowhileCondition())
	}
	return model.EvalDone, nil
}

//todo this shouldn't be accessing the impls

// Evaluates condition set for do while task
func (dw *DoWhileTaskBehavior) evaluateCondition(ctx model.TaskContext, condition expression.Expr) (evalResult model.EvalResult, err error) {
	if t, ok := ctx.(*instance.TaskInst); ok {
		result, err := condition.Eval(getScope(ctx, t))
		if err != nil {
			return model.EvalFail, err
		}
		if result.(bool) {
			delay := ctx.Task().LoopConfig().Delay()
			if delay > 0 {
				ctx.FlowLogger().Infof("Dowhile Task[%s] execution delaying for %d milliseconds...", ctx.Task().ID(), delay)
				time.Sleep(time.Duration(delay) * time.Millisecond)
			}
			ctx.FlowLogger().Infof("Task[%s] repeating as doWhile condition evaluated to true", ctx.Task().ID())
			return model.EvalRepeat, nil
		}
		ctx.FlowLogger().Infof("Task[%s] doWhile condition evaluated to false", ctx.Task().ID())
	}
	return model.EvalDone, nil
}

func getScope(ctx model.TaskContext, t *instance.TaskInst) data.Scope {
	if t.GetWorkingDataScope() != nil {
		return t.GetWorkingDataScope()
	}
	return t.ActivityHost().(data.Scope)
}

func (dw *DoWhileTaskBehavior) updateDoWhileIndex(ctx model.TaskContext) {
	dowhileObj, ok := ctx.GetWorkingData("iteration")
	if !ok {
		dowhileObj = &DoWhile{Index: 1}
	} else {
		dowhileObj.(*DoWhile).Index++
	}
	ctx.SetWorkingData("iteration", dowhileObj)
}

func initIndex(ctx model.TaskContext) {
	dowhileObj, ok := ctx.GetWorkingData("iteration")
	if !ok {
		dowhileObj = &DoWhile{Index: 0}
	}
	ctx.SetWorkingData("iteration", dowhileObj)
}

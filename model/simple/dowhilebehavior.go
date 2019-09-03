package simple

import (
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

// Eval implements model.TaskBehavior.Eval
func (dw *DoWhileTaskBehavior) Eval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	logger := ctx.FlowLogger()

	if ctx.Status() == model.TaskStatusSkipped {
		return model.EvalDone, nil //todo introduce EVAL_SKIP?
	}

	if logger.DebugEnabled() {
		logger.Debugf("Eval do-while Task '%s'", ctx.Task().ID())
	}

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
	return dw.checkDoWhileCondition(ctx)
}

// PostEval implements model.TaskBehavior.PostEval
func (dw *DoWhileTaskBehavior) PostEval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	ctx.FlowLogger().Debugf("PostEval do-while Task '%s'", ctx.Task().ID())

	_, err = ctx.PostEvalActivity()

	if err != nil {
		// check if error returned is retriable
		if errVal, ok := err.(*activity.Error); ok && errVal.Retriable() {
			// check if task is configured to retry on error
			retryData, rerr := getRetryData(ctx)
			if rerr != nil {
				return model.EvalFail, rerr
			}
			if retryData.Count > 0 {
				return retryPostEval(ctx, retryData), nil
			}
		}
		ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
		ctx.FlowLogger().Errorf("Error post evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
		ctx.SetStatus(model.TaskStatusFailed)
		return model.EvalFail, err
	}
	return dw.checkDoWhileCondition(ctx)
}

func (dw *DoWhileTaskBehavior) checkDoWhileCondition(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	if ctx.Task().LoopConfig() != nil && ctx.Task().LoopConfig().DowhileCondition() != nil {
		return dw.evaluateCondition(ctx, ctx.Task().LoopConfig().DowhileCondition())
	}
	return model.EvalDone, nil
}

// Evaluates condition set for do while task
func (dw *DoWhileTaskBehavior) evaluateCondition(ctx model.TaskContext, condition expression.Expr) (evalResult model.EvalResult, err error) {
	if t, ok := ctx.(*instance.TaskInst); ok {
		result, err := condition.Eval(t.ActivityHost().(data.Scope))
		if err != nil {
			return model.EvalFail, err
		}
		if result.(bool) {
			ctx.FlowLogger().Debugf("Task[%s] repeating as do-while condition evaluated to true", ctx.Task().ID())
			return model.EvalRepeat, nil
		}
		ctx.FlowLogger().Debugf("Task[%s] do-while condition evaluated to false", ctx.Task().ID())
	}
	return model.EvalDone, nil
}

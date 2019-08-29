package simple

import (
	"fmt"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
)

// constants for do while
const (
	DoWhileAttr = "_doWhileCondAttr"
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

	done, err := ctx.EvalActivity()

	if err != nil {
		// check if error returned is retriable
		if errVal, ok := err.(*activity.Error); ok && errVal.Retriable() {
			// check if task is configured to retry on error
			retryData, rerr := GetRetryData(ctx, RetryOnErrorAttr)
			if rerr != nil {
				return model.EvalFail, rerr
			}
			if retryData.Count > 0 {
				return DoRetry(ctx, retryData, RetryOnErrorAttr), nil
			}
		}
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
			retryData, rerr := GetRetryData(ctx, RetryOnErrorAttr)
			if rerr != nil {
				return model.EvalFail, rerr
			}
			if retryData.Count > 0 {
				return DoRetry(ctx, retryData, RetryOnErrorAttr), nil
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
	// check if task is configured to repeat on condition match
	condition, err := dw.getDoWhileCondition(ctx)
	if err != nil {
		return model.EvalFail, err
	}
	if len(condition) > 0 {
		return dw.evaluateCondition(ctx, condition)
	}
	return model.EvalDone, nil
}

// Returns condition set for do-while task type
func (dw *DoWhileTaskBehavior) getDoWhileCondition(ctx model.TaskContext) (string, error) {
	if _, ok := ctx.GetWorkingData(DoWhileAttr); !ok {
		// first attempt - condition attribute not set yet
		if d, ok := ctx.GetSetting("do-while"); ok {
			doWhileObj, err := coerce.ToObject(d)
			if err != nil {
				return "", fmt.Errorf("do-while setting is not properly configured")
			}
			condition, _ := doWhileObj["condition"].(string)
			ctx.SetWorkingData(DoWhileAttr, condition)
			return condition, nil
		}
		return "", nil
	}
	// should be set by now
	c, _ := ctx.GetWorkingData(DoWhileAttr)
	return coerce.ToString(c)
}

// Evaluates condition set for do while task
func (dw *DoWhileTaskBehavior) evaluateCondition(ctx model.TaskContext, condition string) (evalResult model.EvalResult, err error) {
	factory := expression.NewFactory(definition.GetDataResolver())
	expr, err := factory.NewExpr(condition)
	if err != nil {
		err = fmt.Errorf("error building expression with condition: %s", condition)
		return model.EvalFail, err
	}

	if t, ok := ctx.(*instance.TaskInst); ok {
		result, err := expr.Eval(t.ActivityHost().(data.Scope))
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

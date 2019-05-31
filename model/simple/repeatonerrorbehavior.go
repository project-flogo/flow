package simple

import (
	"fmt"

	"time"

	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/flow/model"
)

// RepeatOnErrorTaskBehavior implements model.TaskBehavior
type RepeatOnErrorTaskBehavior struct {
	TaskBehavior
}

// Eval implements model.TaskBehavior.Eval
func (rb *RepeatOnErrorTaskBehavior) Eval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {

	logger := ctx.FlowLogger()

	if ctx.Status() == model.TaskStatusSkipped {
		return model.EvalDone, nil //todo introduce EVAL_SKIP?
	}

	if logger.DebugEnabled() {
		logger.Debugf("Eval RepeatOnError Task '%s'", ctx.Task().ID())
	}

	// track retries left for task
	var maxRetries int64
	var retriesLeft int64

	if _, ok := ctx.GetWorkingData("_retriesLeft"); !ok {
		// first attempt - get retry count
		retryObj, _ := ctx.GetSetting("retryCount")
		maxRetries, err = coerce.ToInt64(retryObj)
		if err != nil || maxRetries < 0 {
			err = fmt.Errorf("repeat on error for task '%s' is not properly configured. '%v' is not a valid retry value", ctx.Task().Name(), retryObj)
			logger.Error(err)
			return model.EvalFail, err
		}
		ctx.SetWorkingData("_retriesLeft", maxRetries)
		ctx.SetWorkingData("_maxRetries", maxRetries)
	}

	// should be set by now
	retriesLeftObj, _ := ctx.GetWorkingData("_retriesLeft")
	retriesLeft, _ = coerce.ToInt64(retriesLeftObj)

	maxRetriesObj, _ := ctx.GetWorkingData("_maxRetries")
	maxRetries, _ = coerce.ToInt64(maxRetriesObj)

	if retriesLeft >= 0 {
		done, err := ctx.EvalActivity()

		if err != nil {
			// retry if possible
			evalResult, err := rb.doRetry(ctx, maxRetries, retriesLeft-1, err)
			return evalResult, err
		}

		if !done {
			ctx.SetStatus(model.TaskStatusWaiting)
			return model.EvalWait, nil
		}
	}
	return model.EvalDone, nil
}

// doRetry evaluates if task can be retried
func (rb *RepeatOnErrorTaskBehavior) doRetry(ctx model.TaskContext, maxRetries int64, retriesLeft int64, taskError error) (evalResult model.EvalResult, err error) {
	if retriesLeft >= 0 {
		ctx.FlowLogger().Infof("Task retry attempt (%d)...", maxRetries-retriesLeft)

		// check if retryInterval is set
		retryIntvlObj, _ := ctx.GetSetting("retryInterval")
		retryInterval, err := coerce.ToInt64(retryIntvlObj)
		if err != nil || retryInterval < 0 {
			err = fmt.Errorf("repeat on error for task '%s' is not properly configured. '%v' is not a valid retryInterval value", ctx.Task().Name(), retryIntvlObj)
			ctx.FlowLogger().Error(err)
			return model.EvalFail, err
		}
		if retryInterval > 0 {
			time.Sleep(time.Duration(retryInterval) * time.Millisecond)
		}

		// update retry count
		ctx.SetWorkingData("_retriesLeft", retriesLeft)
		return model.EvalRepeat, nil
	}
	// retries exhausted - throw error
	ref := ctx.Task().ActivityConfig().Ref()
	ctx.FlowLogger().Errorf("Error evaluating activity '%s'[%s] - %s", ref, taskError.Error())
	ctx.SetStatus(model.TaskStatusFailed)
	return model.EvalFail, err
}

// PostEval implements model.TaskBehavior.PostEval
func (rb *RepeatOnErrorTaskBehavior) PostEval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	ctx.FlowLogger().Debugf("PostEval Iterator Task '%s'", ctx.Task().ID())

	_, err = ctx.PostEvalActivity()

	retriesLeftObj, _ := ctx.GetWorkingData("_retriesLeft")
	retriesLeft, _ := coerce.ToInt64(retriesLeftObj)

	maxRetriesObj, _ := ctx.GetWorkingData("_maxRetries")
	maxRetries, _ := coerce.ToInt64(maxRetriesObj)

	if err != nil {
		// retry if possible
		evalResult, err := rb.doRetry(ctx, maxRetries, retriesLeft-1, err)
		return evalResult, err
	}
	return model.EvalDone, nil
}

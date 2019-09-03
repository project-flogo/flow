package simple

import (
	"fmt"
	"time"

	"github.com/project-flogo/flow/model"
)

// constants for retry data
const (
	RetryOnErrorAttr = "_retryOnErrorAttr"
)

// RetryData type
type RetryData struct {
	Count    int
	Interval int
}

// GetRetryData returns retryonerror configuration for a task
func getRetryData(ctx model.TaskContext, attrName string) (retryData *RetryData, err error) {
	if _, ok := ctx.GetWorkingData(attrName); !ok {
		// first attempt - build retry data
		if ctx.Task().LoopConfig() != nil && ctx.Task().LoopConfig().EnabledRetryOnError() {
			retryData := &RetryData{
				Count:    ctx.Task().LoopConfig().RetryOnErrorCount(),
				Interval: ctx.Task().LoopConfig().RetryOnErrorInterval(),
			}
			ctx.SetWorkingData(attrName, retryData)
			return retryData, nil
		}
	}
	// should be set by now
	rd, _ := ctx.GetWorkingData(attrName)
	if retryData, ok := rd.(*RetryData); ok {
		return retryData, nil
	}
	return nil, fmt.Errorf("error getting retry data")
}

// DoRetry retries current task
func retryEval(ctx model.TaskContext, retryData *RetryData, attrName string) (bool, error) {
	ctx.FlowLogger().Debugf("Task[%s] retrying on error. Retries left (%d)...", ctx.Task().ID(), retryData.Count)

	if retryData.Interval > 0 {
		ctx.FlowLogger().Debugf("Task[%s] sleeping for %d milliseconds...", ctx.Task().ID(), retryData.Interval)
		time.Sleep(time.Duration(retryData.Interval) * time.Millisecond)
	}

	// update retry count
	retryData.Count = retryData.Count - 1
	ctx.SetWorkingData(attrName, retryData)
	return evalActivity(ctx)
}

func retryPostEval(ctx model.TaskContext, retryData *RetryData, attrName string) model.EvalResult {
	ctx.FlowLogger().Debugf("Task[%s] retrying on error. Retries left (%d)...", ctx.Task().ID(), retryData.Count)

	if retryData.Interval > 0 {
		ctx.FlowLogger().Debugf("Task[%s] sleeping for %d milliseconds...", ctx.Task().ID(), retryData.Interval)
		time.Sleep(time.Duration(retryData.Interval) * time.Millisecond)
	}

	// update retry count
	retryData.Count = retryData.Count - 1
	ctx.SetWorkingData(attrName, retryData)
	return model.EvalRepeat
}

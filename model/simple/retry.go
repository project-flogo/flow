package simple

import (
	"fmt"
	"time"

	"github.com/project-flogo/flow/model"
)

// constants for retry data
const (
	retryOnErrorAttr = "_retryOnErrorAttr"
)

// RetryData type
type RetryData struct {
	Count    int
	Interval int
}

// getRetryData returns retryonerror configuration for a task
func getRetryData(ctx model.TaskContext) (retryData *RetryData, err error) {
	if _, ok := ctx.GetWorkingData(retryOnErrorAttr); !ok {
		// first attempt - build retry data
		retryData := &RetryData{}
		if ctx.Task().LoopConfig() != nil && ctx.Task().LoopConfig().RetryOnErrorEnabled() {
			retryData.Count = ctx.Task().LoopConfig().RetryOnErrorCount()
			retryData.Interval = ctx.Task().LoopConfig().RetryOnErrorInterval()
			ctx.SetWorkingData(retryOnErrorAttr, retryData)
		}
		return retryData, nil
	}
	// should be set by now
	rd, _ := ctx.GetWorkingData(retryOnErrorAttr)
	if retryData, ok := rd.(*RetryData); ok {
		return retryData, nil
	}
	return nil, fmt.Errorf("error getting retry data")
}

// retryEval retries current task from Eval
func retryEval(ctx model.TaskContext, retryData *RetryData) (bool, error) {
	ctx.FlowLogger().Debugf("Task[%s] retrying on error. Retries left (%d)...", ctx.Task().ID(), retryData.Count)

	if retryData.Interval > 0 {
		ctx.FlowLogger().Debugf("Task[%s] sleeping for %d milliseconds...", ctx.Task().ID(), retryData.Interval)
		time.Sleep(time.Duration(retryData.Interval) * time.Millisecond)
	}

	// update retry count
	retryData.Count = retryData.Count - 1
	ctx.SetWorkingData(retryOnErrorAttr, retryData)
	return evalActivity(ctx)
}

// retryPostEval retries current task from PostEval
//func retryPostEval(ctx model.TaskContext, retryData *RetryData) (bool, error) {
//	ctx.FlowLogger().Debugf("Task[%s] retrying on error. Retries left (%d)...", ctx.Task().ID(), retryData.Count)
//
//	if retryData.Interval > 0 {
//		ctx.FlowLogger().Debugf("Task[%s] sleeping for %d milliseconds...", ctx.Task().ID(), retryData.Interval)
//		time.Sleep(time.Duration(retryData.Interval) * time.Millisecond)
//	}
//
//	// update retry count
//	retryData.Count = retryData.Count - 1
//	ctx.SetWorkingData(retryOnErrorAttr, retryData)
//	return evalActivity(ctx)
//}

package simple

import (
	"errors"
	"fmt"
	"github.com/project-flogo/flow/instance"
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

// getRetryData returns the RetryData for a task
func getRetryData(ctx model.TaskContext) (retryData *RetryData, err error) {

	//assumes task was configured for retry
	rd, ok := ctx.GetWorkingData(retryOnErrorAttr)

	if !ok {
		// first attempt - build retry data
		retryData = &RetryData{}

		retryCfg := ctx.Task().RetryOnErrConfig()
		if t, ok := ctx.(*instance.TaskInst); ok {
			retryData.Count, err = retryCfg.Count(getScope(t))
			if err != nil {
				return nil, err
			}
			retryData.Interval, err = retryCfg.Interval(getScope(t))
			if err != nil {
				return nil, err
			}
		}
		ctx.SetWorkingData(retryOnErrorAttr, retryData)
	} else {
		retryData, ok = rd.(*RetryData)
		if !ok {
			return nil, fmt.Errorf("error getting retry data")
		}
	}

	return retryData, nil
}

// retryEval retries current task from Eval
func retryEval(ctx model.TaskContext, retryData *RetryData) (bool, error) {

	if retryData == nil {
		return false, errors.New("Retry Data not specified.")
	}

	ctx.FlowLogger().Infof("Task[%s] retrying on error. Retries left (%d)...", ctx.Task().ID(), retryData.Count)

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

package simple

import (
	"fmt"
	"time"

	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/flow/model"
)

// constants for retry data
const (
	RetryOnErrorAttr = "_retryOnErrorAttr"
)

// RetryData type
type RetryData struct {
	Count    int64
	Interval int64
}

// GetRetryData returns retryonerror configuration for a task
func GetRetryData(ctx model.TaskContext, attrName string) (retryData *RetryData, err error) {
	if _, ok := ctx.GetWorkingData(attrName); !ok {
		// first attempt - build retry data
		if retrySetting, ok := ctx.GetSetting("retryonerror"); ok {
			retryObj, err := coerce.ToObject(retrySetting)
			if err != nil {
				return nil, fmt.Errorf("retry setting is not properly configured")
			}
			return buildRetryData(ctx, retryObj, attrName)
		}
		return &RetryData{}, nil
	}
	// should be set by now
	rd, _ := ctx.GetWorkingData(attrName)
	if retryData, ok := rd.(*RetryData); ok {
		return retryData, nil
	}
	err = fmt.Errorf("error getting retry data")
	return nil, err
}

func buildRetryData(ctx model.TaskContext, retryObj map[string]interface{}, attrName string) (retryData *RetryData, err error) {
	retryCount, _ := coerce.ToInt64(retryObj["count"])
	retryInterval, _ := coerce.ToInt64(retryObj["interval"])

	if retryCount < 0 {
		err = fmt.Errorf("retry on error for task '%s' is not properly configured. '%d' is not a valid value for count", ctx.Task().Name(), retryCount)
		ctx.FlowLogger().Error(err)
		return nil, err
	}

	if retryInterval < 0 {
		err = fmt.Errorf("retry on error for task '%s' is not properly configured. '%d' is not a valid value for interval", ctx.Task().Name(), retryInterval)
		ctx.FlowLogger().Error(err)
		return nil, err
	}

	retryData = &RetryData{
		Count:    retryCount,
		Interval: retryInterval,
	}
	ctx.SetWorkingData(attrName, retryData)
	return retryData, nil
}

// DoRetry retries current task
func DoRetry(ctx model.TaskContext, retryData *RetryData, attrName string) (evalResult model.EvalResult) {
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

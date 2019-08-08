package simple

import (
	"fmt"
	"strings"
	"time"

	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
)

// constants for repeat data
const (
	ErrorRepeatData  = "_errorRepeatData"
	OnCondRepeatData = "_onCondRepeatData"
)

// RepeatData type
type RepeatData struct {
	Count     int64
	Interval  int64
	Condition string
}

// GetRepeatData returns onerror or oncond configuration for a task
func GetRepeatData(ctx model.TaskContext, attrName string) (repeatData *RepeatData, err error) {
	if _, ok := ctx.GetWorkingData(attrName); !ok {
		// first attempt - build repeat data
		if repeatSetting, ok := ctx.GetSetting("repeat"); ok {
			repeatObj, err := coerce.ToObject(repeatSetting)
			if err != nil {
				return nil, fmt.Errorf("repeat setting is not properly configured")
			}
			var repeatChild map[string]interface{}
			if strings.HasPrefix(attrName, "_error") {
				repeatChild, _ = coerce.ToObject(repeatObj["onerror"])
			} else {
				repeatChild, _ = coerce.ToObject(repeatObj["oncondition"])
			}
			return buildRepeatData(ctx, repeatChild, attrName)
		}
		return &RepeatData{}, nil
	}
	// should be set by now
	rd, _ := ctx.GetWorkingData(attrName)
	if repeatData, ok := rd.(*RepeatData); ok {
		return repeatData, nil
	}
	err = fmt.Errorf("error getting repeat data")
	return nil, err
}

func buildRepeatData(ctx model.TaskContext, repeatObj map[string]interface{}, attrName string) (repeatData *RepeatData, err error) {
	repeatCount, _ := coerce.ToInt64(repeatObj["count"])
	repeatInterval, _ := coerce.ToInt64(repeatObj["interval"])

	if repeatCount < 0 {
		err = fmt.Errorf("repeat on error for task '%s' is not properly configured. '%d' is not a valid value for count", ctx.Task().Name(), repeatCount)
		ctx.FlowLogger().Error(err)
		return nil, err
	}

	if repeatInterval < 0 {
		err = fmt.Errorf("repeat on error for task '%s' is not properly configured. '%d' is not a valid value for interval", ctx.Task().Name(), repeatInterval)
		ctx.FlowLogger().Error(err)
		return nil, err
	}

	repeatCondition, _ := coerce.ToString(repeatObj["expression"])
	repeatData = &RepeatData{
		Count:     repeatCount,
		Interval:  repeatInterval,
		Condition: repeatCondition,
	}
	ctx.SetWorkingData(attrName, repeatData)
	return repeatData, nil
}

// EvaluateExpression evaluates condition for repeat
func EvaluateExpression(ctx model.TaskContext, repeatData *RepeatData) (evalResult model.EvalResult, err error) {
	cond := repeatData.Condition
	factory := expression.NewFactory(definition.GetDataResolver())
	expr, err := factory.NewExpr(cond)
	if err != nil {
		err = fmt.Errorf("error building expression with condition: %s", cond)
		return model.EvalFail, err
	}

	if t, ok := ctx.(*instance.TaskInst); ok {
		result, err := expr.Eval(t.ActivityHost().(data.Scope))
		if err != nil {
			return model.EvalFail, err
		}
		if result.(bool) {
			ctx.FlowLogger().Debugf("Repeat on condition evaluated to true for task [%s]", ctx.Task().ID())
			return DoRepeat(ctx, repeatData, OnCondRepeatData)
		}
		ctx.FlowLogger().Debugf("Repeat on condition evaluated to false for task [%s]", ctx.Task().ID())
	}
	return model.EvalDone, nil
}

// DoRepeat retries current task
func DoRepeat(ctx model.TaskContext, repeatData *RepeatData, attrName string) (evalResult model.EvalResult, err error) {
	if strings.EqualFold(attrName, OnCondRepeatData) {
		ctx.FlowLogger().Infof("Task[%s] repeating on condition match. Repeat count (%d)...", ctx.Task().ID(), repeatData.Count+1)
	} else {
		ctx.FlowLogger().Infof("Task[%s] retrying on error. Retries left (%d)...", ctx.Task().ID(), repeatData.Count)
	}

	if repeatData.Interval > 0 {
		time.Sleep(time.Duration(repeatData.Interval) * time.Millisecond)
	}

	// update repeat count
	if strings.EqualFold(attrName, OnCondRepeatData) {
		repeatData.Count = repeatData.Count + 1
	} else {
		repeatData.Count = repeatData.Count - 1
	}
	ctx.SetWorkingData(attrName, repeatData)
	return model.EvalRepeat, nil
}

package simple

import (
	"fmt"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/flow/instance"
	"reflect"
	"time"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/flow/model"
)

// SimpleIteratorTask implements model.TaskBehavior
type IteratorTaskBehavior struct {
	TaskBehavior
}

// Eval implements model.TaskBehavior.Eval
func (tb *IteratorTaskBehavior) Eval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {

	logger := ctx.FlowLogger()

	if ctx.Status() == model.TaskStatusSkipped {
		return model.EvalDone, nil //todo introduce EVAL_SKIP?
	}

	if logger.DebugEnabled() {
		logger.Debugf("Eval Iterator Task '%s'", ctx.Task().ID())
	}

	var itx Iterator

	itxAttr, ok := ctx.GetWorkingData("_iterator")
	iterationAttr, _ := ctx.GetWorkingData("iteration")

	if ok {
		itx = itxAttr.(Iterator)
	} else {

		var iterateOn interface{}
		iterate := ctx.Task().LoopConfig().GetIterate()
		switch t := iterate.(type) {
		case expression.Expr:
			iterateOn, err = t.Eval(ctx.(*instance.TaskInst).ActivityHost().(data.Scope))
			if err != nil {
				return model.EvalFail, err
			}
		default:
			iterateOn = t
		}

		if iterateOn == nil {
			//todo if iterateOn is not defined, what should we do?
			//just skip for now
			return model.EvalDone, nil
		}

		switch t := iterateOn.(type) {
		case string:
			count, err := coerce.ToInt(iterateOn)
			if err != nil {
				err = fmt.Errorf("iterator '%s' not properly configured. '%s' is not a valid iterate value", ctx.Task().Name(), iterateOn)
				logger.Error(err)
				return model.EvalFail, err
			}
			itx = NewIntIterator(count)
		case int64:
			itx = NewIntIterator(int(t))
		case float64:
			itx = NewIntIterator(int(t))
		case int:
			count := iterateOn.(int)
			itx = NewIntIterator(count)
		case map[string]interface{}:
			itx = NewObjectIterator(t)
		case []interface{}:
			itx = NewArrayIterator(t)
		default:

			val := reflect.ValueOf(iterateOn)
			rt := val.Kind()

			if rt == reflect.Array || rt == reflect.Slice {
				itx = NewReflectIterator(val)
			} else {
				err = fmt.Errorf("iterator '%s' not properly configured. '%+v' is not a valid iterate value", ctx.Task().Name(), iterateOn)
				logger.Error(err)
				return model.EvalFail, err
			}
		}

		itxAttr = itx
		ctx.SetWorkingData("_iterator", itxAttr)

		iteration := map[string]interface{}{
			"key":   nil,
			"value": nil,
			"index": 0,
		}

		iterationAttr = iteration
		ctx.SetWorkingData("iteration", iteration)
	}

	shouldIterate := itx.next()

	if shouldIterate {
		if logger.DebugEnabled() {
			logger.Debugf("Repeat:%t, Key:%v, Value:%v", shouldIterate, itx.Key(), itx.Value())
		}

		iteration, _ := iterationAttr.(map[string]interface{})
		iteration["key"] = itx.Key()
		iteration["value"] = itx.Value()
		iteration["index"] = itx.Index()

		done, err := evalActivity(ctx)
		if err != nil {
			ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
			ctx.FlowLogger().Errorf("Error evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
			ctx.SetStatus(model.TaskStatusFailed)
			return model.EvalFail, err
		}

		//Wait for deply
		delay := ctx.Task().LoopConfig().Delay()
		if delay > 0 {
			ctx.FlowLogger().Infof("Iterate Task[%s] execution delaying for %d milliseconds...", ctx.Task().ID(), delay)
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		if !done {
			ctx.SetStatus(model.TaskStatusWaiting)
			return model.EvalWait, nil
		}
		evalResult = model.EvalRepeat

	} else {
		evalResult = model.EvalDone
	}

	return evalResult, nil
}

// PostEval implements model.TaskBehavior.PostEval
func (tb *IteratorTaskBehavior) PostEval(ctx model.TaskContext) (evalResult model.EvalResult, err error) {
	ctx.FlowLogger().Debugf("PostEval Iterator Task '%s'", ctx.Task().ID())
	_, err = ctx.PostEvalActivity()

	if err != nil {
		ref := activity.GetRef(ctx.Task().ActivityConfig().Activity)
		ctx.FlowLogger().Errorf("Error post evaluating activity '%s'[%s] - %s", ctx.Task().ID(), ref, err.Error())
		ctx.SetStatus(model.TaskStatusFailed)
		return model.EvalFail, err
	}
	ctx.SetStatus(model.TaskStatusDone)

	itxAttr, _ := ctx.GetWorkingData("_iterator")
	itx := itxAttr.(Iterator)

	if itx.HasNext() {
		return model.EvalRepeat, nil
	}

	return model.EvalDone, nil
}

//func getIterateValue(ctx model.TaskContext) (value interface{}, set bool) {
//
//	value, set = ctx.Task().GetSetting("iterate")
//	if !set {
//		return nil, false
//	}
//
//	strVal, ok := value.(string)
//	if ok {
//		val, err := ctx.Resolve(strVal)
//		if err != nil {
//			ctx.FlowLogger().Errorf("Get iterate value failed, due to %s", err.Error())
//			return nil, false
//		}
//		return val, true
//	}
//
//	return value, true
//}

///////////////////////////////////
// Iterators

type Iterator interface {
	Key() interface{}
	Value() interface{}
	next() bool
	HasNext() bool
	Index() int
}

type ArrayIterator struct {
	current int
	data    []interface{}
}

func (itx *ArrayIterator) Key() interface{} {
	return itx.current
}

func (itx *ArrayIterator) Value() interface{} {
	return itx.data[itx.current]
}

func (itx *ArrayIterator) HasNext() bool {
	if itx.current >= len(itx.data) {
		return false
	}
	return true
}

func (itx *ArrayIterator) Index() int {
	return itx.current
}

func (itx *ArrayIterator) next() bool {
	itx.current++
	if itx.current >= len(itx.data) {
		return false
	}
	return true
}

func NewArrayIterator(data []interface{}) *ArrayIterator {
	return &ArrayIterator{data: data, current: -1}
}

type IntIterator struct {
	current int
	count   int
}

func (itx *IntIterator) Key() interface{} {
	return itx.current
}

func (itx *IntIterator) Value() interface{} {
	return itx.current
}

func (itx *IntIterator) HasNext() bool {
	if itx.current >= itx.count {
		return false
	}
	return true
}

func (itx *IntIterator) Index() int {
	return itx.current
}

func (itx *IntIterator) next() bool {
	itx.current++
	if itx.current >= itx.count {
		return false
	}
	return true
}

func NewIntIterator(count int) *IntIterator {
	return &IntIterator{count: count, current: -1}
}

type ObjectIterator struct {
	current int
	keyMap  map[int]string
	data    map[string]interface{}
}

func (itx *ObjectIterator) Key() interface{} {
	return itx.keyMap[itx.current]
}

func (itx *ObjectIterator) Value() interface{} {
	key := itx.keyMap[itx.current]
	return itx.data[key]
}

func (itx *ObjectIterator) HasNext() bool {
	if itx.current >= len(itx.data) {
		return false
	}
	return true
}

func (itx *ObjectIterator) next() bool {
	itx.current++
	if itx.current >= len(itx.data) {
		return false
	}
	return true
}

func (itx *ObjectIterator) Index() int {
	return itx.current
}

func NewObjectIterator(data map[string]interface{}) *ObjectIterator {
	keyMap := make(map[int]string, len(data))
	i := 0
	for key := range data {
		keyMap[i] = key
		i++
	}

	return &ObjectIterator{keyMap: keyMap, data: data, current: -1}
}

type ReflectIterator struct {
	current int
	val     reflect.Value
}

func (itx *ReflectIterator) Key() interface{} {
	return itx.current
}

func (itx *ReflectIterator) Value() interface{} {
	e := itx.val.Index(itx.current)
	return e.Interface()
}

func (itx *ReflectIterator) HasNext() bool {
	if itx.current >= itx.val.Len() {
		return false
	}
	return true
}

func (itx *ReflectIterator) next() bool {
	itx.current++
	if itx.current >= itx.val.Len() {
		return false
	}
	return true
}

func (itx *ReflectIterator) Index() int {
	return itx.current
}

func NewReflectIterator(val reflect.Value) *ReflectIterator {
	return &ReflectIterator{val: val, current: -1}
}

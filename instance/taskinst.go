package instance

import (
	"fmt"
	"runtime/debug"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/model"
)

func NewTaskInst(inst *Instance, task *definition.Task) *TaskInst {
	var taskInst TaskInst

	taskInst.flowInst = inst
	taskInst.task = task
	taskInst.taskID = task.ID()

	if log.CtxLoggingEnabled() {
		taskInst.logger = log.ChildLoggerWithFields(task.ActivityConfig().Logger, log.String("flowId", inst.ID()))

	} else {
		taskInst.logger = task.ActivityConfig().Logger
	}

	return &taskInst
}

type TaskInst struct {
	flowInst *Instance
	task     *definition.Task
	status   model.TaskStatus

	workingData *WorkingDataScope

	inputs   map[string]interface{}
	outputs  map[string]interface{}
	settings map[string]interface{}

	logger log.Logger

	//needed for serialization
	taskID string
}

/////////////////////////////////////////
// activity.Context Implementation

func (ti *TaskInst) ActivityHost() activity.Host {
	return ti.flowInst
}

// Name implements activity.Context.Name method
func (ti *TaskInst) Name() string {
	return ti.task.Name()
}

// GetInput implements activity.Context.GetInput
func (ti *TaskInst) GetInput(name string) interface{} {

	val, found := ti.inputs[name]
	if found {
		return val
	}

	return nil
}

// SetOutput implements activity.Context.SetOutput
func (ti *TaskInst) SetOutput(name string, value interface{}) error {

	if ti.logger.DebugEnabled() {
		ti.logger.Debugf("Task[%s] - Set Output: %s = %v", ti.taskID, name, value)
	}

	if ti.outputs == nil {
		ti.outputs = make(map[string]interface{})
	}

	ti.outputs[name] = value

	return nil
}

// GetInputObject implements activity.Context.GetInputObject
func (ti *TaskInst) GetInputObject(input data.StructValue) error {
	err := input.FromMap(ti.inputs)
	return err
}

// SetOutputObject implements activity.Context.SetOutputObject
func (ti *TaskInst) SetOutputObject(output data.StructValue) error {
	ti.outputs = output.ToMap()
	return nil
}

func (ti *TaskInst) GetSharedTempData() map[string]interface{} {
	//todo implement
	return nil
}

func (ti *TaskInst) Logger() log.Logger {
	return ti.logger
}

/////////////////////////////////////////
// model.TaskContext Implementation

// Status implements flow.TaskContext.GetState
func (ti *TaskInst) Status() model.TaskStatus {
	return ti.status
}

// SetStatus implements flow.TaskContext.SetStatus
func (ti *TaskInst) SetStatus(status model.TaskStatus) {
	ti.status = status
	ti.flowInst.master.ChangeTracker.trackTaskData(ti.flowInst.subFlowId, &TaskInstChange{ChgType: CtUpd, ID: ti.task.ID(), TaskInst: ti})
}

//func (ti *TaskInst) HasWorkingData() bool {
//	return ti.workingData != nil
//}

//func (ti *TaskInst) Resolve(toResolve string) (value interface{}, err error) {
//	//Support expression mapping
//
//	//return exprmapper.GetMappingValue(toResolve, ti.flowInst, definition.GetDataResolver())
//	return nil, nil
//}

func (ti *TaskInst) SetWorkingData(key string, value interface{}) error {
	if ti.workingData == nil {
		ti.workingData = NewWorkingDataScope(ti.flowInst)
	}
	return ti.workingData.SetWorkingValue(key, value)
}

func (ti *TaskInst) GetWorkingData(key string) (interface{}, bool) {
	if ti.workingData == nil {
		return nil, false
	}

	return ti.workingData.GetWorkingValue(key)
}

// Task implements model.TaskContext.Task, by returning the Task associated with this
// TaskInst object
func (ti *TaskInst) Task() *definition.Task {
	return ti.task
}

func (ti *TaskInst) FlowLogger() log.Logger {
	return ti.flowInst.logger
}

/////////////////////////////////////////

// GetFromLinkInstances implements model.TaskContext.GetFromLinkInstances
func (ti *TaskInst) GetFromLinkInstances() []model.LinkInstance {

	links := ti.task.FromLinks()

	numLinks := len(links)

	if numLinks > 0 {
		linkCtxs := make([]model.LinkInstance, numLinks)

		for i, link := range links {
			linkCtxs[i], _ = ti.flowInst.FindOrCreateLinkData(link)
		}
		return linkCtxs
	}

	return nil
}

// GetToLinkInstances implements model.TaskContext.GetToLinkInstances,
func (ti *TaskInst) GetToLinkInstances() []model.LinkInstance {

	links := ti.task.ToLinks()

	numLinks := len(links)

	if numLinks > 0 {
		linkCtxs := make([]model.LinkInstance, numLinks)

		for i, link := range links {
			linkCtxs[i], _ = ti.flowInst.FindOrCreateLinkData(link)
		}
		return linkCtxs
	}

	return nil
}

// EvalLink implements activity.ActivityContext.EvalLink method
func (ti *TaskInst) EvalLink(link *definition.Link) (result bool, err error) {

	defer func() {
		if r := recover(); r != nil {
			ti.logger.Warnf("Unhandled Error evaluating link '%s' : %v\n", link.ID(), r)
			ti.logger.Debugf("StackTrace: %s", debug.Stack())

			if err != nil {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	if expr := link.Expr(); expr != nil {
		result, err := expr.Eval(ti.flowInst)
		if err != nil {
			return false, err
		}

		return coerce.ToBool(result)
	}

	return true, nil
}

// HasActivity implements activity.ActivityContext.HasActivity method
func (ti *TaskInst) HasActivity() bool {
	return ti.task.ActivityConfig().Activity != nil
}

// EvalActivity implements activity.ActivityContext.EvalActivity method
func (ti *TaskInst) EvalActivity() (done bool, evalErr error) {

	defer func() {
		if r := recover(); r != nil {

			ref := activity.GetRef(ti.task.ActivityConfig().Activity)
			ti.logger.Warnf("Unhandled Error executing activity '%s'[%s] : %v\n", ti.task.ID(), ref, r)

			if ti.logger.DebugEnabled() {
				ti.logger.Debugf("StackTrace: %s", debug.Stack())
			}

			if evalErr == nil {
				evalErr = NewActivityEvalError(ti.task.Name(), "unhandled", fmt.Sprintf("%v", r))
				done = false
			}
		}
		if evalErr != nil {
			ti.logger.Errorf("Execution failed for Activity[%s] in Flow[%s] - %s", ti.task.ID(), ti.flowInst.flowDef.Name(), evalErr.Error())
		}
	}()

	eval := true

	if ti.task.ActivityConfig().InputMapper() != nil {

		err := applyInputMapper(ti)

		if err != nil {

			evalErr = NewActivityEvalError(ti.task.Name(), "mapper", err.Error())
			return false, evalErr
		}
	}

	eval = applyInputInterceptor(ti)

	if eval {

		act := ti.task.ActivityConfig().Activity
		done, evalErr = act.Eval(ti)

		if evalErr != nil {
			e, ok := evalErr.(*activity.Error)
			if ok {
				e.SetActivityName(ti.task.Name())
			}

			return false, evalErr
		}
	} else {
		done = true
	}

	if done {

		applyOutputInterceptor(ti)

		if ti.task.ActivityConfig().OutputMapper() != nil {

			appliedMapper, err := applyOutputMapper(ti)

			if err != nil {
				evalErr = NewActivityEvalError(ti.task.Name(), "mapper", err.Error())
				return done, evalErr
			}

			if !appliedMapper && !ti.task.IsScope() {

				ti.logger.Debug("Mapper not applied")
			}
		}
	}

	return done, nil
}

// EvalActivity implements activity.ActivityContext.EvalActivity method
func (ti *TaskInst) PostEvalActivity() (done bool, evalErr error) {

	act := ti.task.ActivityConfig().Activity

	defer func() {
		if r := recover(); r != nil {
			ti.logger.Warnf("Unhandled Error executing activity '%s'[%s] : %v\n", ti.task.Name(), activity.GetRef(act), r)

			if ti.logger.DebugEnabled() {
				ti.logger.Debugf("StackTrace: %s", debug.Stack())
			}

			if evalErr == nil {
				evalErr = NewActivityEvalError(ti.task.Name(), "unhandled", fmt.Sprintf("%v", r))
				done = false
			}
		}
		if evalErr != nil {
			ti.logger.Errorf("Execution failed for Activity[%s] in Flow[%s] - %s", ti.task.Name(), ti.flowInst.flowDef.Name(), evalErr.Error())
		}
	}()

	aa, ok := act.(activity.AsyncActivity)
	done = true

	if ok {
		done, evalErr = aa.PostEval(ti, nil)

		if evalErr != nil {
			e, ok := evalErr.(*activity.Error)
			if ok {
				e.SetActivityName(ti.task.Name())
			}

			return false, evalErr
		}
	}

	if done {

		if ti.task.ActivityConfig().OutputMapper() != nil {
			applyOutputInterceptor(ti)

			appliedMapper, err := applyOutputMapper(ti)

			if err != nil {
				evalErr = NewActivityEvalError(ti.task.Name(), "mapper", err.Error())
				return done, evalErr
			}

			if !appliedMapper && !ti.task.IsScope() {

				ti.logger.Debug("Mapper not applied")
			}
		}
	}

	return done, nil
}

func (ti *TaskInst) GetSetting(name string) (value interface{}, exists bool) {

	if ti.settings == nil {
		return nil, false
	}

	value, exists = ti.settings[name]
	return value, exists
}

//// FlowReply is used to reply to the Flow Host with the results of the execution
//func (ti *TaskInst) FlowReply(replyData map[string]interface{}, err error) {
//	//ignore
//}
//
//// FlowReturn is used to indicate to the Flow Host that it should complete and return the results of the execution
//func (ti *TaskInst) FlowReturn(returnData map[string]interface{}, err error) {
//
//	if err != nil {
//		for name, value := range returnData {
//			ti.SetWorkingData(name, value)
//		}
//	}
//}

func (ti *TaskInst) appendErrorData(err error) {

	switch e := err.(type) {
	case *definition.LinkExprError:
		ti.flowInst.SetValue("_E.type", "link_expr")
		ti.flowInst.SetValue("_E.message", err.Error())
		ti.flowInst.SetValue("_E.data", nil)
		ti.flowInst.SetValue("_E.code", "")
		ti.flowInst.SetValue("_E.activity", ti.taskID)
	case *activity.Error:
		ti.flowInst.SetValue("_E.type", "activity")
		ti.flowInst.SetValue("_E.message", err.Error())
		ti.flowInst.SetValue("_E.data", e.Data())
		ti.flowInst.SetValue("_E.code", e.Code())

		if e.ActivityName() != "" {
			ti.flowInst.SetValue("_E.activity", e.ActivityName())
		} else {
			ti.flowInst.SetValue("_E.activity", ti.taskID)
		}
	case *ActivityEvalError:
		ti.flowInst.SetValue("_E.activity", e.TaskName())
		ti.flowInst.SetValue("_E.message", err.Error())
		ti.flowInst.SetValue("_E.type", e.Type())
		ti.flowInst.SetValue("_E.data", nil)
		ti.flowInst.SetValue("_E.code", "")
	default:
		ti.flowInst.SetValue("_E.activity", ti.taskID)
		ti.flowInst.SetValue("_E.message", err.Error())
		ti.flowInst.SetValue("_E.type", "unknown")
		ti.flowInst.SetValue("_E.data", nil)
		ti.flowInst.SetValue("_E.code", "")
	}

	//todo add case for *dataMapperError & *activity.Error
}

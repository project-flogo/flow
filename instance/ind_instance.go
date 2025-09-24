package instance

import (
	"context"
	"errors"
	"fmt"
	"github.com/project-flogo/core/activity"
	"os"
	"strconv"
	"time"

	"github.com/project-flogo/flow/state"

	coresupport "github.com/project-flogo/core/engine/support"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/support/trace"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/model"
	flowsupport "github.com/project-flogo/flow/support"
)

type IndependentInstance struct {
	*Instance

	id            string
	stepID        int
	workItemQueue *support.SyncQueue //todo: change to faster non-threadsafe queue
	wiCounter     int

	trackingChanges bool
	changeTracker   ChangeTracker

	flowModel   *model.FlowModel
	patch       *flowsupport.Patch
	interceptor *coresupport.Interceptor

	subflowCtr int
	subflows   map[int]*Instance
	startTime  time.Time
	//Instance recorder
	instRecorder *stateInstanceRecorder
}

const (
	flowCtxPrefix  = "_fctx."
	flowName       = "FlowName"
	flowId         = "FlowId"
	parentFlowName = "ParentFlowName"
	parentFlowId   = "ParentFlowId"
	traceId        = "TraceId"
	spanId         = "SpanId"
	appName        = "AppName"
	appVersion     = "AppVersion"
	debugMode      = "DebugMode"
)

// New creates a new Flow Instance from the specified Flow
func NewIndependentInstance(instanceID string, flowURI string, flow *definition.Definition, instRecorder *stateInstanceRecorder, logger log.Logger) (*IndependentInstance, error) {
	var err error
	inst := &IndependentInstance{}
	inst.Instance = &Instance{}
	inst.attrs = make(map[string]interface{})
	inst.master = inst
	inst.id = instanceID
	inst.stepID = 0
	inst.workItemQueue = support.NewSyncQueue()
	inst.flowDef = flow
	inst.flowURI = flowURI
	inst.flowModel, err = getFlowModel(flow)
	if err != nil {
		return nil, err
	}
	inst.logger = logger

	inst.status = model.FlowStatusNotStarted
	if os.Getenv("FLOGO_FLOW_SM_ENDPOINT") != "" {
		inst.trackingChanges = true
	}
	inst.changeTracker = NewInstanceChangeTracker(inst.id, 0)
	inst.changeTracker.FlowCreated(inst)

	inst.taskInsts = make(map[string]*TaskInst)
	inst.linkInsts = make(map[int]*LinkInst)

	inst.instRecorder = instRecorder
	return inst, nil
}

func (inst *IndependentInstance) SetInstanceRecorder(stateRecorder *stateInstanceRecorder) {
	inst.instRecorder = stateRecorder
}

func (inst *IndependentInstance) newEmbeddedInstance(taskInst *TaskInst, flowURI string, flow *definition.Definition, ctx context.Context, cancelFunc context.CancelFunc) *Instance {
	inst.subflowCtr++

	embeddedInst := &Instance{}
	embeddedInst.attrs = make(map[string]interface{})
	embeddedInst.subflowId = inst.subflowCtr
	embeddedInst.master = inst
	embeddedInst.host = taskInst
	embeddedInst.flowDef = flow
	embeddedInst.status = model.FlowStatusNotStarted
	embeddedInst.taskInsts = make(map[string]*TaskInst)
	embeddedInst.linkInsts = make(map[int]*LinkInst)
	embeddedInst.flowURI = flowURI
	embeddedInst.logger = inst.logger
	embeddedInst.goContext = ctx
	embeddedInst.cancelFunc = cancelFunc

	if trace.Enabled() {
		tc, _ := trace.GetTracer().StartTrace(embeddedInst.SpanConfig(), taskInst.traceContext) //TODO handle error
		embeddedInst.tracingCtx = tc
	}

	if inst.subflows == nil {
		inst.subflows = make(map[int]*Instance)
	}
	inst.subflows[embeddedInst.subflowId] = embeddedInst

	inst.changeTracker.SubflowCreated(embeddedInst)
	//inst.ChangeTracker.SubFlowChange(taskInst.flowInst.subFlowId, CtAdd, embeddedInst.subFlowId, "")

	return embeddedInst
}

func (inst *IndependentInstance) UpdateStartTime() {
	inst.startTime = time.Now().UTC()
}

func (inst *IndependentInstance) ExecutionTime() time.Duration {
	return time.Since(inst.startTime)
}

func (inst *IndependentInstance) GetTimeoutContext() context.Context {
	return inst.goContext
}

func (inst *IndependentInstance) GetFlowState(inputs map[string]interface{}) *state.FlowState {
	retData, _ := inst.GetReturnData()

	return &state.FlowState{
		UserId:         flowsupport.GetUserName(),
		AppName:        flowsupport.GetAppName(),
		AppVersion:     flowsupport.GetAppVerison(),
		HostId:         flowsupport.GetHostId(),
		FlowName:       inst.Name(),
		FlowInstanceId: inst.id,
		FlowInputs:     inputs,
		FlowOutputs:    retData,
		FlowStats:      string(convertFlowStatus(inst.status)),
		StartTime:      inst.startTime,
		EndTime:        time.Now().UTC(),
	}
}

func (inst *IndependentInstance) Start(startAttrs map[string]interface{}) bool {
	return inst.startInstance(inst.Instance, startAttrs)
}

func (inst *IndependentInstance) startEmbedded(embedded *Instance, startAttrs map[string]interface{}) error {

	if embedded.master != inst {
		return errors.New("embedded instance is not from this independent instance")
	}

	inst.startInstance(embedded, startAttrs)
	return nil
}

func (inst *IndependentInstance) startInstance(toStart *Instance, startAttrs map[string]interface{}) bool {

	//Set the flow Name and Flow Id for the current flow.
	_ = toStart.SetValue(flowCtxPrefix+flowName, toStart.Name())
	_ = toStart.SetValue(flowCtxPrefix+flowId, toStart.ID())
	_ = toStart.SetValue(flowCtxPrefix+appName, flowsupport.GetAppName())
	_ = toStart.SetValue(flowCtxPrefix+appVersion, flowsupport.GetAppVerison())
	if inst.interceptor != nil {
		_ = toStart.SetValue(debugMode, true)
	}

	// If tracing is enabled, inject traceId and spanId in flow context
	if trace.Enabled() {
		_ = toStart.SetValue(flowCtxPrefix+traceId, toStart.tracingCtx.TraceID())
		_ = toStart.SetValue(flowCtxPrefix+spanId, toStart.tracingCtx.SpanID())
	} else {
		_ = toStart.SetValue(flowCtxPrefix+traceId, "")
		_ = toStart.SetValue(flowCtxPrefix+spanId, "")
	}

	if inst.logger.DebugEnabled() {
		inst.logger.Debugf("Flow Name: %s", toStart.Name())
		inst.logger.Debugf("Flow Id: %s", toStart.ID())
		if trace.Enabled() {
			inst.logger.Debugf("Trace Id: %s", toStart.tracingCtx.TraceID())
			inst.logger.Debugf("Span Id: %s", toStart.tracingCtx.SpanID())
		}
	}

	// If the flow is a sub flow then the flow name and flow id  of the parent flow of the current flow needs to be set.
	// The parent flow can be main flow or sub flow.
	if toStart.host != nil {
		hostInstance := toStart.host.(*TaskInst)

		if inst.logger.DebugEnabled() {
			inst.logger.Debugf("Parent Flow Name: %s", hostInstance.flowInst.Name())
			inst.logger.Debugf("Parent Flow Id: %s", hostInstance.flowInst.ID())
		}
		//Set the flow Name and Flow Id for the current flow.
		_ = toStart.SetValue(flowCtxPrefix+parentFlowName, hostInstance.flowInst.Name())
		_ = toStart.SetValue(flowCtxPrefix+parentFlowId, hostInstance.flowInst.ID())
	} else {
		// Set Empty in case of Main Flow.
		_ = toStart.SetValue(flowCtxPrefix+parentFlowName, "")
		_ = toStart.SetValue(flowCtxPrefix+parentFlowId, "")
	}

	md := toStart.flowDef.Metadata()

	if md != nil && md.Input != nil {

		if toStart.attrs == nil {
			toStart.attrs = make(map[string]interface{}, len(md.Input))
		}
		for name, value := range md.Input {
			if value != nil {
				toStart.attrs[name] = value.Value()
				inst.changeTracker.AttrChange(toStart.subflowId, name, value)
			} else {
				toStart.attrs[name] = nil
			}
		}
	} else {
		if toStart.attrs == nil {
			toStart.attrs = make(map[string]interface{}, len(md.Input))
		}
	}

	for name, value := range startAttrs {
		toStart.attrs[name] = value
		inst.changeTracker.AttrChange(toStart.subflowId, name, value)
	}

	toStart.SetStatus(model.FlowStatusActive)

	flowBehavior := inst.flowModel.GetFlowBehavior()
	ok, taskEntries := flowBehavior.Start(toStart)

	if ok {
		err := inst.enterTasks(toStart, taskEntries)
		if err != nil {
			//todo review how we should handle an error encountered here
			log.RootLogger().Errorf("encountered error when entering tasks: %v", err)
		}
	}

	return ok
}

func (inst *IndependentInstance) ApplyPatch(patch *flowsupport.Patch) {
	if inst.patch == nil {
		inst.patch = patch
		inst.patch.Init()
	}
}

func (inst *IndependentInstance) ApplyInterceptor(interceptor *coresupport.Interceptor) {
	if inst.interceptor == nil {
		inst.interceptor = interceptor
		inst.interceptor.Init()
	}
}

func (inst *IndependentInstance) HasInterceptor() bool {
	return inst.interceptor != nil
}

func (inst *IndependentInstance) GetInterceptor() *coresupport.Interceptor {
	return inst.interceptor
}

// GetChanges returns the Change Tracker object
func (inst *IndependentInstance) GetChanges() ChangeTracker {
	return inst.changeTracker
}

// ResetChanges resets an changes that were being tracked
func (inst *IndependentInstance) ResetChanges() {

}

// StepID returns the current step ID of the Flow Instance
func (inst *IndependentInstance) StepID() int {
	return inst.stepID
}

func (inst *IndependentInstance) DoStep() bool {

	hasNext := false

	inst.ResetChanges()

	inst.stepID++

	if inst.status == model.FlowStatusActive {

		// get item to be worked on
		item, ok := inst.workItemQueue.Pop()

		if ok {
			//dev logging
			//logger.Debug("Retrieved item from Flow Instance work queue")

			workItem := item.(*WorkItem)

			// get the corresponding behavior
			behavior := inst.flowModel.GetDefaultTaskBehavior()
			if typeID := workItem.taskInst.task.TypeID(); typeID != "" {
				behavior = inst.flowModel.GetTaskBehavior(typeID)
			}

			// track the fact that the work item was removed from the queue
			inst.changeTracker.WorkItemRemoved(workItem)

			if workItem.taskInst.flowInst.goContext != nil {
				inst.execTaskWithContext(workItem.taskInst.flowInst.goContext, workItem.taskInst.flowInst.cancelFunc, behavior, workItem.taskInst)
			} else {
				inst.execTask(behavior, workItem.taskInst)
			}

			hasNext = true
		} else {
			// dev logging
			//logger.Debug("Flow Instance work queue empty")
		}
	}

	return hasNext
}

// DoStepWithContext executes a single step of the flow with context cancellation support

func (inst *IndependentInstance) scheduleEval(taskInst *TaskInst) {

	inst.wiCounter++

	workItem := NewWorkItem(inst.wiCounter, taskInst)
	inst.logger.Debugf("Scheduling task '%s'", taskInst.task.ID())

	inst.workItemQueue.Push(workItem)

	// track the fact that the work item was added to the queue
	inst.changeTracker.WorkItemAdded(workItem)
}

// execTask executes the specified Work Item of the Flow Instance
func (inst *IndependentInstance) execTask(behavior model.TaskBehavior, taskInst *TaskInst) {

	defer func() {
		if r := recover(); r != nil {

			err := fmt.Errorf("unhandled Error executing task '%s' : %v", taskInst.task.ID(), r)
			inst.logger.Error(err)
			if taskInst.traceContext != nil {
				_ = trace.GetTracer().FinishTrace(taskInst.traceContext, err)
			}

			// todo: useful for debugging
			//logger.Debugf("StackTrace: %s", debug.Stack())

			if !taskInst.flowInst.isHandlingError {

				taskInst.appendErrorData(NewActivityEvalError(taskInst.task.Name(), "unhandled", err.Error()))
				inst.HandleGlobalError(taskInst.flowInst, err)
			}
			// else what should we do?
		}
	}()

	var err error
	var evalResult model.EvalResult

	if taskInst.status == model.TaskStatusWaiting {
		evalResult, err = behavior.PostEval(taskInst)
	} else if taskInst.status == model.TaskStatusSkipped {
		return
	} else {

		evalResult, err = behavior.Eval(taskInst)
	}

	if err != nil {
		//taskInst.returnError = err
		inst.handleTaskError(behavior, taskInst, err)
		return
	}

	switch evalResult {
	case model.EvalDone:
		//taskInst.SetStatus(model.TaskStatusDone)
		inst.handleTaskDone(behavior, taskInst)
	case model.EvalSkip:
		//taskInst.SetStatus(model.TaskStatusSkipped)
		inst.handleTaskDone(behavior, taskInst)
	case model.EvalWait:
		taskInst.SetStatus(model.TaskStatusWaiting)
	case model.EvalFail:
		taskInst.SetStatus(model.TaskStatusFailed)
		if taskInst.traceContext != nil {
			_ = trace.GetTracer().FinishTrace(taskInst.traceContext, taskInst.returnError)
		}
	case model.EvalRepeat:
		taskInst.UpdateTaskToTracker()
		if taskInst.traceContext != nil {
			// Finish previous span
			_ = trace.GetTracer().FinishTrace(taskInst.traceContext, nil)
			taskInst.counter++
			taskInst.id = taskInst.taskID + "-" + strconv.Itoa(taskInst.counter)
			// Reset span
			taskInst.traceContext = nil
		}
		//task needs to iterate or retry
		inst.scheduleEval(taskInst)
	}
}

func (inst *IndependentInstance) execTaskWithContext(ctx context.Context, cancelFunc context.CancelFunc, behavior model.TaskBehavior, taskInst *TaskInst) {

	defer func() {
		if r := recover(); r != nil {

			err := fmt.Errorf("unhandled Error executing task '%s' : %v", taskInst.task.ID(), r)
			inst.logger.Error(err)
			if taskInst.traceContext != nil {
				_ = trace.GetTracer().FinishTrace(taskInst.traceContext, err)
			}

			// todo: useful for debugging
			//logger.Debugf("StackTrace: %s", debug.Stack())

			if !taskInst.flowInst.isHandlingError {

				taskInst.appendErrorData(NewActivityEvalError(taskInst.task.Name(), "unhandled", err.Error()))
				inst.HandleGlobalError(taskInst.flowInst, err)
			}
			// else what should we do?
		}
	}()
	inst.logger.Debugf("executing task '%s' in context %v", taskInst.task.ID(), ctx)
	// Check for cancellation before task evaluation
	select {
	case <-ctx.Done():
		cancelFunc()
		inst.logger.Debugf(" context timeout before task '%s' evaluation ", taskInst.task.ID())
		inst.handleTaskCancelled(behavior, taskInst, nil, ctx)
		return
	default:
	}

	var err error
	var evalResult model.EvalResult

	if taskInst.status == model.TaskStatusWaiting {
		// Run PostEval in goroutine with context
		resultChan := make(chan struct {
			result model.EvalResult
			err    error
		}, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					resultChan <- struct {
						result model.EvalResult
						err    error
					}{model.EvalFail, fmt.Errorf("panic in PostEval: %v", r)}
				}
			}()

			result, evalErr := behavior.PostEval(taskInst)
			if evalErr != nil {
				inst.addActivityToCoverage(taskInst, evalErr)
			} else {
				inst.addActivityToCoverage(taskInst, nil)
			}
			resultChan <- struct {
				result model.EvalResult
				err    error
			}{result, evalErr}
		}()

		select {
		case <-ctx.Done():
			cancelFunc()
			inst.logger.Debugf(" context timeout during post evaluation of task '%s' ", taskInst.task.ID())
			inst.handleTaskCancelled(behavior, taskInst, nil, ctx)
			return
		case res := <-resultChan:
			evalResult = res.result
			err = res.err
		}

	} else if taskInst.status == model.TaskStatusSkipped {
		return
	} else {
		// Run Eval in goroutine with context
		resultChan := make(chan struct {
			result model.EvalResult
			err    error
		}, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					resultChan <- struct {
						result model.EvalResult
						err    error
					}{model.EvalFail, fmt.Errorf("panic in Eval: %v", r)}
				}
			}()

			result, evalErr := behavior.Eval(taskInst)
			if evalErr != nil {
				inst.addActivityToCoverage(taskInst, evalErr)
			} else {
				inst.addActivityToCoverage(taskInst, nil)
			}
			resultChan <- struct {
				result model.EvalResult
				err    error
			}{result, evalErr}
		}()

		select {
		case <-ctx.Done():
			cancelFunc()
			inst.logger.Debugf(" context timeout during eval of task '%s' ", taskInst.task.ID())
			inst.handleTaskCancelled(behavior, taskInst, nil, ctx)
			return
		case res := <-resultChan:
			evalResult = res.result
			err = res.err
		}
	}

	if err != nil {
		// Check for cancellation after error
		select {
		case <-ctx.Done():
			cancelFunc()
			inst.logger.Debugf(" context timeout after eval error of task '%s' ", taskInst.task.ID())
			inst.handleTaskCancelled(behavior, taskInst, nil, ctx)
			return
		default:
		}
		inst.handleTaskError(behavior, taskInst, err)
		return
	}

	// Check for cancellation after task evaluation
	select {
	case <-ctx.Done():
		cancelFunc()
		inst.logger.Debugf(" context timeout after eval of task '%s' ", taskInst.task.ID())
		inst.handleTaskCancelled(behavior, taskInst, nil, ctx)
		return
	default:
	}

	switch evalResult {
	case model.EvalDone:
		//taskInst.SetStatus(model.TaskStatusDone)
		inst.handleTaskDone(behavior, taskInst)
	case model.EvalSkip:
		//taskInst.SetStatus(model.TaskStatusSkipped)
		inst.handleTaskDone(behavior, taskInst)
	case model.EvalWait:
		taskInst.SetStatus(model.TaskStatusWaiting)
	case model.EvalFail:
		taskInst.SetStatus(model.TaskStatusFailed)
		if taskInst.traceContext != nil {
			_ = trace.GetTracer().FinishTrace(taskInst.traceContext, taskInst.returnError)
		}
	case model.EvalRepeat:
		taskInst.UpdateTaskToTracker()
		if taskInst.traceContext != nil {
			// Finish previous span
			_ = trace.GetTracer().FinishTrace(taskInst.traceContext, nil)
			taskInst.counter++
			taskInst.id = taskInst.taskID + "-" + strconv.Itoa(taskInst.counter)
			// Reset span
			taskInst.traceContext = nil
		}
		//task needs to iterate or retry
		inst.scheduleEval(taskInst)
	}

}

// handleTaskDone handles the completion of a task in the Flow Instance
func (inst *IndependentInstance) handleTaskDone(taskBehavior model.TaskBehavior, taskInst *TaskInst) {
	notifyFlow := false
	propagateSkip := false
	var taskEntries []*model.TaskEntry
	var err error

	containerInst := taskInst.flowInst

	if taskInst.Status() == model.TaskStatusSkipped {
		notifyFlow, taskEntries, propagateSkip = taskBehavior.Skip(taskInst)

		if propagateSkip {
			notifyFlow = inst.propagateSkip(taskEntries, containerInst)
		}
	} else {
		notifyFlow, taskEntries, err = taskBehavior.Done(taskInst)
		if taskInst.traceContext != nil {
			_ = trace.GetTracer().FinishTrace(taskInst.traceContext, nil)
		}
		inst.addActivityToCoverage(taskInst, nil)
	}

	if err != nil {
		err = fmt.Errorf("error handling task done transition for task [%s] in flow [%s] : %s", taskInst.Name(), inst.flowDef.Name(), err.Error())
		taskInst.appendErrorData(err)
		inst.HandleGlobalError(containerInst, err)
		return
	}

	flowDone := false
	task := taskInst.Task()

	if notifyFlow {
		flowBehavior := inst.flowModel.GetFlowBehavior()
		flowDone = flowBehavior.TaskDone(containerInst)
	}

	if flowDone || containerInst.forceCompletion {
		//flow completed or return was called explicitly, so lets complete the flow
		flowBehavior := inst.flowModel.GetFlowBehavior()
		flowBehavior.Done(containerInst)
		flowDone = true
		containerInst.SetStatus(model.FlowStatusCompleted)

		if containerInst != inst.Instance {
			//not top level flow so we have to schedule next step
			// Complete subflow trace
			if containerInst.tracingCtx != nil {
				_ = trace.GetTracer().FinishTrace(containerInst.tracingCtx, nil)
			}

			// spawned from task instance
			host, ok := containerInst.host.(*TaskInst)

			if ok {
				host.SetOutputs(containerInst.returnData)
				// Reset error if any
				host.returnError = nil
				//Sub flow done
				if inst.HasInterceptor() {
					subFlowCoverage := inst.interceptor.GetSubFlowCoverageEntry(containerInst.ID())
					subFlowCoverage.Outputs = containerInst.returnData
					inst.interceptor.AddToSubFlowCoverageMap(containerInst.ID(), subFlowCoverage)
				}

				containerInst.master.GetChanges().SubflowDone(containerInst)
				inst.scheduleEval(host)
			}

			//if containerInst.isHandlingError {
			//	//was the error handler, so directly under instance
			//	host,ok := containerInst.host.(*EmbeddedInstance)
			//	if ok {
			//		host.SetStatus(model.FlowStatusCompleted)
			//		host.returnData = containerInst.returnData
			//		host.returnError = containerInst.returnError
			//	}
			//	//todo if not a task inst, what should we do?
			//} else {
			//	// spawned from task instance
			//
			//	//todo if not a task inst, what should we do?
			//}
			delete(inst.subflows, containerInst.subflowId)
		} else {
			containerInst.master.GetChanges().FlowDone(inst)
		}

	} else {

		if !propagateSkip {
			// not done, so enter tasks specified by the Done behavior call
			err := inst.enterTasks(containerInst, taskEntries)
			if err != nil {
				//todo review how we should handle an error encountered here
				log.RootLogger().Errorf("encountered error when entering tasks: %v", err)
			}
		}
	}

	// task is done, so we can release it
	containerInst.releaseTask(task)
}

func (inst *IndependentInstance) propagateSkip(taskEntries []*model.TaskEntry, activeInst *Instance) bool {

	if len(taskEntries) == 0 {
		//no entries, so essentially a notifyFlow
		return true
	}

	notify := false
	for _, entry := range taskEntries {
		newTaskInst, _ := activeInst.FindOrCreateTaskInst(entry.Task)
		newTaskInst.id = newTaskInst.taskID

		taskToEnterBehavior := inst.flowModel.GetTaskBehavior(entry.Task.TypeID())
		enterResult := taskToEnterBehavior.Enter(newTaskInst)
		if enterResult == model.ERSkip {
			_, toSkipEntries, _ := taskToEnterBehavior.Skip(newTaskInst)
			if len(toSkipEntries) > 0 {
				shouldNotify := inst.propagateSkip(toSkipEntries, activeInst)
				notify = notify || shouldNotify
			} else {
				notify = true
			}
		}
	}

	return notify
}

// handleTaskError handles the completion of a task in the Flow Instance
func (inst *IndependentInstance) handleTaskError(taskBehavior model.TaskBehavior, taskInst *TaskInst, err error) {

	if taskInst.traceContext != nil {
		_ = trace.GetTracer().FinishTrace(taskInst.traceContext, err)
	}
	// Set task status to failed for subflow activity
	taskInst.SetStatus(model.TaskStatusFailed)

	inst.addActivityToCoverage(taskInst, err)
	handled, taskEntries := taskBehavior.Error(taskInst, err)

	containerInst := taskInst.flowInst

	if handled {
		//Add error details to scope
		taskInst.setTaskError(err)
		if len(taskEntries) != 0 {
			err := inst.enterTasks(containerInst, taskEntries)
			if err != nil {
				//todo review how we should handle an error encountered here
				log.RootLogger().Errorf("encountered error when entering tasks: %v", err)
			}
		}

		containerInst.releaseTask(taskInst.Task())
	} else {
		if containerInst.isHandlingError {
			//fail
			containerInst.SetStatus(model.FlowStatusFailed)

			if containerInst != inst.Instance {

				// Complete SubflowCreated trace
				if containerInst.tracingCtx != nil {
					_ = trace.GetTracer().FinishTrace(containerInst.tracingCtx, err)
				}

				if containerInst != nil && containerInst.master != nil {
					containerInst.master.RecordState(time.Now().UTC())
				}
				// spawned from task instance
				host, ok := containerInst.host.(*TaskInst)

				if ok {
					host.returnError = err
					inst.scheduleEval(host)
				}
			} else {
				taskInst.appendErrorData(err)
			}

		} else {
			taskInst.appendErrorData(err)
			inst.HandleGlobalError(containerInst, err)
		}
		return
	}

}

func (inst *IndependentInstance) handleTaskCancelled(taskBehavior model.TaskBehavior, taskInst *TaskInst, err error, ctx context.Context) {

	inst.logger.Debugf("handleTaskCancelled for task '%s' ", taskInst.Task().Name())
	message := fmt.Sprintf("Flow execution timed out during execution of activity %s", taskInst.Task().Name())

	attr, isLoop := taskInst.GetWorkingData("iterateIndex")
	index := ""
	if isLoop {

		index = attr.(string)
		inst.logger.Debugf("task %s was running in loop at index '%s' ", taskInst.Task().Name(), index)
		message = fmt.Sprintf("Flow execution timed out during execution of activity %s running in loop at index %s", taskInst.Task().Name(), index)
	}
	var timeoutValue string
	if val := ctx.Value("timeoutSeconds"); val != nil {
		timeoutValue = val.(string)
		inst.logger.Debugf("task timeout value %s ", val)

	}

	data := map[string]interface{}{
		"timeoutInFlow":         taskInst.flowInst.flowDef.Name(),
		"timeoutInActivity":     taskInst.task.Name(),
		"timeoutInMilliSeconds": timeoutValue,
	}
	err = activity.NewActivityError(message, "SUBFLOW-001", activity.TimeoutError, data)
	if taskInst.traceContext != nil {
		_ = trace.GetTracer().FinishTrace(taskInst.traceContext, err)
	}
	// Set task status to failed for subflow activity
	taskInst.SetStatus(model.TaskStatusCancelled)
	inst.logger.Debugf("task set to cancelled for task '%s' ", taskInst.Task().Name())

	containerInst := taskInst.flowInst
	//containerInst.SetStatus(model.FlowStatusCancelled)
	taskInst.appendErrorData(err)
	inst.HandleCancelError(containerInst, err)

	return

}

// HandleGlobalError handles instance errors
func (inst *IndependentInstance) HandleGlobalError(containerInst *Instance, err error) {

	if containerInst.isHandlingError {
		//todo: log error information
		containerInst.SetStatus(model.FlowStatusFailed)
		return
	}

	containerInst.isHandlingError = true

	flowBehavior := inst.flowModel.GetFlowBehavior()

	//not currently handling error, so check if it has an error handler
	if containerInst.flowDef.GetErrorHandler() != nil {

		// todo: should we clear out the existing workitem queue for items from containerInst?
		inst.logger.Infof("Running flow [%s] failed, now handle error in the error handler", inst.flowDef.Name())
		//clear existing instances
		inst.taskInsts = make(map[string]*TaskInst)

		taskEntries := flowBehavior.StartErrorHandler(containerInst)
		err := inst.enterTasks(containerInst, taskEntries)
		if err != nil {
			//todo review how we should handle an error encountered here
			log.RootLogger().Errorf("encountered error when entering tasks: %v", err)
		}

	} else {
		// Print error message if no error handler
		inst.logger.Error(err)
		containerInst.SetStatus(model.FlowStatusFailed)

		if containerInst != inst.Instance {

			// Complete SubflowCreated trace
			if containerInst.tracingCtx != nil {
				_ = trace.GetTracer().FinishTrace(containerInst.tracingCtx, err)
			}

			if containerInst != nil && containerInst.master != nil {
				containerInst.master.RecordState(time.Now().UTC())
			}
			// spawned from task instance
			host, ok := containerInst.host.(*TaskInst)

			if ok {
				host.returnError = err
				inst.scheduleEval(host)
			}
		} else {
			inst.returnError = err
		}
	}
}

func (inst *IndependentInstance) HandleCancelError(containerInst *Instance, err error) {

	// Print error message if no error handler
	inst.logger.Error(err)
	containerInst.SetStatus(model.FlowStatusCancelled)
	inst.logger.Debugf("HandleCancelError for task and flow set to cancelled '%s' ", containerInst.Name())

	if containerInst != inst.Instance {

		// Complete SubflowCreated trace
		if containerInst.tracingCtx != nil {
			_ = trace.GetTracer().FinishTrace(containerInst.tracingCtx, err)
		}

		if containerInst != nil && containerInst.master != nil {
			containerInst.master.RecordState(time.Now().UTC())
		}
		// spawned from task instance
		host, ok := containerInst.host.(*TaskInst)

		if ok {
			host.returnError = err
			inst.scheduleEval(host)
		}
	} else {
		inst.returnError = err
	}

}

func (inst *IndependentInstance) enterTasks(activeInst *Instance, taskEntries []*model.TaskEntry) error {
	for _, taskEntry := range taskEntries {
		//logger.Debugf("EnterTask - TaskEntry: %v", taskEntry)
		behavior := inst.flowModel.GetTaskBehavior(taskEntry.Task.TypeID())
		taskInst, _ := activeInst.FindOrCreateTaskInst(taskEntry.Task)
		taskInst.id = taskInst.taskID
		enterResult := behavior.Enter(taskInst)
		if enterResult == model.EREval {
			err := applySettingsMapper(taskInst)
			if err != nil {
				return err
			}
			inst.scheduleEval(taskInst)
		} else if enterResult == model.ERSkip {
			inst.handleTaskDone(behavior, taskInst)
		}
	}

	return nil
}

func (inst *IndependentInstance) addActivityToCoverage(taskInst *TaskInst, err error) {

	if !inst.HasInterceptor() {
		return
	}
	var errorObj map[string]interface{}
	if err != nil {
		errorObj = taskInst.getErrorObject(err)
	}

	var coverage coresupport.ActivityCoverage
	var outputs interface{} = nil
	if inst.GetInterceptor().CollectIO {
		outputs = taskInst.outputs

		if cfg := taskInst.Task().LoopConfig(); cfg != nil && cfg.Accumulate() {
			outputs, _ = taskInst.flowInst.GetValue("_A." + taskInst.Task().ID())
		}

		if taskInst.outputs == nil {
			if taskInst.flowInst.returnData != nil {
				outputs = taskInst.flowInst.returnData
			}
		}
		coverage = coresupport.ActivityCoverage{
			ActivityName: taskInst.taskID,
			LinkFrom:     inst.getLinks(taskInst.GetFromLinkInstances()),
			LinkTo:       inst.getLinks(taskInst.GetToLinkInstances()),
			Inputs:       taskInst.inputs,
			Outputs:      outputs,
			Error:        errorObj,
			FlowName:     taskInst.flowInst.Name(),
			IsMainFlow:   !taskInst.flowInst.isHandlingError,
			FlowId:       taskInst.flowInst.ID(),
		}
	} else {
		coverage = coresupport.ActivityCoverage{
			ActivityName: taskInst.taskID,
			FlowName:     taskInst.flowInst.Name(),
			IsMainFlow:   !inst.isHandlingError,
		}
	}

	inst.interceptor.AddToActivityCoverage(coverage)
}

func (inst *IndependentInstance) addSubFlowToCoverage(subFlowName, subFlowActivity, hostFlowName string, hostInstanceId string, instanceId string, inputs map[string]interface{}, isLoop bool, index string) {

	if !inst.HasInterceptor() {
		return
	}

	coverage := coresupport.SubFlowCoverage{
		HostFlow:        hostFlowName,
		SubFlowActivity: subFlowActivity,
		SubFlowName:     subFlowName,
		SubFlowID:       instanceId,
		Inputs:          inputs,
		HostFlowID:      hostInstanceId,
		IsLoop:          isLoop,
		Index:           index,
	}

	inst.interceptor.AddToSubFlowCoverage(coverage)
	inst.interceptor.AddToSubFlowCoverageMap(instanceId, &coverage)
}

func (inst *IndependentInstance) getLinks(instances []model.LinkInstance) []string {
	var names []string
	for _, linkInst := range instances {
		names = append(names, linkInst.Link().Label())
	}
	return names
}

//////////////////////////////////////////////////////////////////

// WorkItem describes an item of work (event for a Task) that should be executed on Step
type WorkItem struct {
	ID       int       `json:"id"`
	taskInst *TaskInst `json:"-"`

	TaskID    string `json:"taskID"`
	SubFlowID int    `json:"subFlowId"`
}

// NewWorkItem constructs a new WorkItem for the specified TaskInst
func NewWorkItem(id int, taskInst *TaskInst) *WorkItem {

	var workItem WorkItem

	workItem.ID = id
	workItem.taskInst = taskInst
	workItem.TaskID = taskInst.task.ID()
	workItem.SubFlowID = taskInst.flowInst.subflowId

	return &workItem
}

func NewActivityEvalError(taskName string, errorType string, errorText string) *ActivityEvalError {
	return &ActivityEvalError{taskName: taskName, errType: errorType, errText: errorText}
}

type ActivityEvalError struct {
	taskName string
	errType  string
	errText  string
}

func (e *ActivityEvalError) TaskName() string {
	return e.taskName
}

func (e *ActivityEvalError) Type() string {
	return e.errType
}

func (e *ActivityEvalError) Error() string {
	return e.errText
}

//////////////
// todo fix the following

func getFlowModel(flow *definition.Definition) (*model.FlowModel, error) {
	if flow.ModelID() == "" {
		return model.Default(), nil
	}
	return model.Get(flow.ModelID())
	//todo if model not found, should throw error

}

// // Restart indicates that this FlowInstance was restarted
func (inst *IndependentInstance) Restart(logger log.Logger, id string, initStepId int) error {
	inst.id = id
	inst.logger = logger

	var err error
	inst.flowDef, _, err = flowsupport.GetDefinition(inst.flowURI)
	if err != nil {
		return err
	}
	if inst.flowDef == nil {
		return errors.New("unable to resolve flow: " + inst.flowURI)
	}

	inst.flowModel, err = getFlowModel(inst.flowDef)
	if err != nil {
		return err
	}
	inst.master = inst
	inst.init(inst.Instance)

	inst.changeTracker = NewInstanceChangeTracker(inst.id, initStepId)
	inst.changeTracker.FlowCreated(inst)
	// Set flow status to active
	inst.SetStatus(model.FlowStatusActive)
	if initStepId+1 > 0 {
		// Restart from activity. Read the item from queue
		step, ok := inst.workItemQueue.Pop()
		if ok {
			wi, ok := step.(*WorkItem)
			if ok {
				if wi.taskInst != nil {
					// Update the status
					wi.taskInst.SetStatus(model.TaskStatusReady)
					// Add item back to queue
					inst.workItemQueue.Push(step)
				}
			}
		}
	}

	if initStepId+1 > 0 {
		// Restart from activity. Read the item from queue
		step, ok := inst.workItemQueue.Pop()
		if ok {
			wi, ok := step.(*WorkItem)
			if ok {
				if wi.taskInst != nil {
					// Update the status
					wi.taskInst.SetStatus(model.TaskStatusReady)
					// Add item back to queue
					inst.workItemQueue.Push(step)
				}
			}
		}
	}

	return nil
}

func (inst *IndependentInstance) init(flowInst *Instance) {

	for _, taskInst := range flowInst.taskInsts {
		initTaskInst(taskInst, flowInst, nil)
		//v.flowInst = flowInst
		//v.task = flowInst.flowDef.GetTask(v.taskID)
	}

	for _, linkInst := range flowInst.linkInsts {
		linkInst.flowInst = flowInst
		linkInst.link = flowInst.flowDef.GetLink(linkInst.id)
	}
}

func (inst *IndependentInstance) SetTracingContext(tracingCtx trace.TracingContext) {
	inst.tracingCtx = tracingCtx
}

func (inst *Instance) SpanConfig() trace.Config {
	config := trace.Config{}
	config.Operation = inst.Name()
	config.Logger = inst.Logger()
	config.Tags = make(map[string]interface{})
	config.Tags["flow_id"] = inst.ID()
	config.Tags["flow_name"] = inst.Name()
	if inst.master != nil && inst.master.id != inst.ID() {
		config.Tags["parent_flow_id"] = inst.master.ID()
		config.Tags["parent_flow_name"] = inst.master.Name()
	}
	return config
}

func (inst *IndependentInstance) CurrentStep(reset bool) *state.Step {
	step := inst.changeTracker.ExtractStep(reset)
	return step
}

func (inst *IndependentInstance) Snapshot() *state.Snapshot {
	fs := &state.Snapshot{
		SnapshotBase: &state.SnapshotBase{},
		Id:           inst.id,
	}

	populateBaseSnapshot(inst.Instance, fs.SnapshotBase)

	if len(inst.subflows) > 0 {
		fs.Subflows = make([]*state.Subflow, 0, len(inst.subflows))
		for id, subflow := range inst.subflows {
			sfs := state.Subflow{
				SnapshotBase: &state.SnapshotBase{},
				Id:           id,
				TaskId:       subflow.host.(*TaskInst).taskID,
			}
			populateBaseSnapshot(subflow, sfs.SnapshotBase)
		}
	}
	return fs
}

func populateBaseSnapshot(inst *Instance, base *state.SnapshotBase) {

	base.FlowURI = inst.flowURI
	base.Status = int(inst.status)

	if len(inst.attrs) > 0 {
		base.Attrs = make(map[string]interface{}, len(inst.attrs))
		for name, value := range inst.attrs {
			base.Attrs[name] = value
		}
	}

	if len(inst.taskInsts) > 0 {
		base.Tasks = make([]*state.Task, 0, len(inst.taskInsts))
		for id, task := range inst.taskInsts {
			base.Tasks = append(base.Tasks, &state.Task{Id: id, Status: int(task.status)})
		}
	}

	if len(inst.linkInsts) > 0 {
		base.Links = make([]*state.Link, 0, len(inst.linkInsts))
		for id, link := range inst.linkInsts {
			base.Links = append(base.Links, &state.Link{Id: id, Status: int(link.status)})
		}
	}
}

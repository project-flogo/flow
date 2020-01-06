package instance

import (
	"errors"
	"fmt"
	"github.com/project-flogo/flow/state"
	"strconv"

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

	//trackingChanges bool
	changeTracker ChangeTracker

	flowModel   *model.FlowModel
	patch       *flowsupport.Patch
	interceptor *flowsupport.Interceptor

	subflowCtr int
	subflows   map[int]*Instance
}

// New creates a new Flow Instance from the specified Flow
func NewIndependentInstance(instanceID string, flowURI string, flow *definition.Definition, logger log.Logger) (*IndependentInstance, error) {
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
	inst.changeTracker = NewInstanceChangeTracker(inst.id)
	inst.changeTracker.FlowCreated(inst)

	inst.taskInsts = make(map[string]*TaskInst)
	inst.linkInsts = make(map[int]*LinkInst)

	return inst, nil
}

func (inst *IndependentInstance) newEmbeddedInstance(taskInst *TaskInst, flowURI string, flow *definition.Definition) *Instance {

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

	md := toStart.flowDef.Metadata()

	if md != nil && md.Input != nil {

		toStart.attrs = make(map[string]interface{}, len(md.Input))

		for name, value := range md.Input {
			if value != nil {
				toStart.attrs[name] = value.Value()
				inst.changeTracker.AttrChange(toStart.subflowId, name, value)
			} else {
				toStart.attrs[name] = nil
			}
		}
	} else {
		toStart.attrs = make(map[string]interface{}, len(startAttrs))
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

func (inst *IndependentInstance) ApplyInterceptor(interceptor *flowsupport.Interceptor) {
	if inst.interceptor == nil {
		inst.interceptor = interceptor
		inst.interceptor.Init()
	}
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

			inst.execTask(behavior, workItem.taskInst)

			hasNext = true
		} else {
			// dev logging
			//logger.Debug("Flow Instance work queue empty")
		}
	}

	return hasNext
}

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
	}

	if err != nil {
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
				//if the flow failed, set the error
				for name, value := range containerInst.returnData {
					//todo review how we should handle an error encountered here
					_ = host.SetOutput(name, value)
				}

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

			// flow has completed so remove it
			delete(inst.subflows, containerInst.subflowId)
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

				// spawned from task instance
				host, ok := containerInst.host.(*TaskInst)

				if ok {
					behavior := inst.flowModel.GetDefaultTaskBehavior()
					if typeID := host.task.TypeID(); typeID != "" {
						behavior = inst.flowModel.GetTaskBehavior(typeID)
					}
					inst.handleTaskError(behavior, host, err)
				}
			}

		} else {
			taskInst.appendErrorData(err)
			inst.HandleGlobalError(containerInst, err)
		}
		return
	}

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

		//clear existing instances
		inst.taskInsts = make(map[string]*TaskInst)

		taskEntries := flowBehavior.StartErrorHandler(containerInst)
		err := inst.enterTasks(containerInst, taskEntries)
		if err != nil {
			//todo review how we should handle an error encountered here
			log.RootLogger().Errorf("encountered error when entering tasks: %v", err)
		}

	} else {

		containerInst.SetStatus(model.FlowStatusFailed)

		if containerInst != inst.Instance {

			// Complete SubflowCreated trace
			if containerInst.tracingCtx != nil {
				_ = trace.GetTracer().FinishTrace(containerInst.tracingCtx, err)
			}

			// spawned from task instance
			host, ok := containerInst.host.(*TaskInst)

			if ok {
				behavior := inst.flowModel.GetDefaultTaskBehavior()
				if typeID := host.task.TypeID(); typeID != "" {
					behavior = inst.flowModel.GetTaskBehavior(typeID)
				}

				inst.handleTaskError(behavior, host, err)

				//fail the task

				//inst.scheduleEval(host)
			}
		} else {
			inst.returnError = err
		}
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
			taskInst.SetStatus(model.TaskStatusReady)
			inst.scheduleEval(taskInst)
		} else if enterResult == model.ERSkip {
			inst.handleTaskDone(behavior, taskInst)
		}
	}

	return nil
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

//// Restart indicates that this FlowInstance was restarted
func (inst *IndependentInstance) Restart(logger log.Logger, id string) error {
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

	inst.changeTracker = NewInstanceChangeTracker(inst.id)
	inst.changeTracker.FlowCreated(inst)

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
	return inst.changeTracker.ExtractStep(reset)
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
				TaskId:       inst.host.(TaskInst).taskID,
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

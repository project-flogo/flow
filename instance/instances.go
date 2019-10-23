package instance

import (
	"errors"
	"fmt"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
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
	ChangeTracker   *InstanceChangeTracker

	subFlowCtr  int
	flowModel   *model.FlowModel
	patch       *flowsupport.Patch
	interceptor *flowsupport.Interceptor

	subFlows map[int]*Instance
}

// New creates a new Flow Instance from the specified Flow
func NewIndependentInstance(instanceID string, flowURI string, flow *definition.Definition, logger log.Logger) (*IndependentInstance, error) {
	var err error
	inst := &IndependentInstance{}
	inst.Instance = &Instance{}
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
	inst.ChangeTracker = NewInstanceChangeTracker()

	inst.taskInsts = make(map[string]*TaskInst)
	inst.linkInsts = make(map[int]*LinkInst)

	return inst, nil
}

func (inst *IndependentInstance) newEmbeddedInstance(taskInst *TaskInst, flowURI string, flow *definition.Definition) *Instance {

	inst.subFlowCtr++

	embeddedInst := &Instance{}
	embeddedInst.subFlowId = inst.subFlowCtr
	embeddedInst.master = inst
	embeddedInst.host = taskInst
	embeddedInst.flowDef = flow
	embeddedInst.status = model.FlowStatusNotStarted
	embeddedInst.taskInsts = make(map[string]*TaskInst)
	embeddedInst.linkInsts = make(map[int]*LinkInst)
	embeddedInst.flowURI = flowURI
	embeddedInst.logger = inst.logger

	if inst.subFlows == nil {
		inst.subFlows = make(map[int]*Instance)
	}
	inst.subFlows[embeddedInst.subFlowId] = embeddedInst

	inst.ChangeTracker.SubFlowChange(taskInst.flowInst.subFlowId, CtAdd, embeddedInst.subFlowId, "")

	return embeddedInst
}

func (inst *IndependentInstance) startEmbedded(embedded *Instance, startAttrs map[string]interface{}) error {

	if embedded.master != inst {
		return errors.New("embedded instance is not from this independent instance")
	}

	//attrs := make(map[string]data.TypedValue, len(startAttrs))
	//
	//inputMd := embedded.flowDef.Metadata().Input
	//
	//for name, value := range startAttrs {
	//
	//	if tv, exists := inputMd[name]; exists {
	//		attrs[name] = data.NewTypedValue(tv.Type(), value)
	//	}
	//}

	embedded.attrs = startAttrs

	inst.startInstance(embedded)
	return nil
}

func (inst *IndependentInstance) Start(startAttrs map[string]interface{}) bool {

	md := inst.flowDef.Metadata()

	if md != nil && md.Input != nil {

		inst.attrs = make(map[string]interface{}, len(md.Input))

		for name, value := range md.Input {
			if value != nil {
				inst.attrs[name] = value.Value()
			} else {
				inst.attrs[name] = nil
			}
		}
	} else {
		inst.attrs = make(map[string]interface{}, len(startAttrs))
	}

	for name, value := range startAttrs {
		inst.attrs[name] = value
	}

	return inst.startInstance(inst.Instance)
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
func (inst *IndependentInstance) GetChanges() *InstanceChangeTracker {
	return inst.ChangeTracker
}

// ResetChanges resets an changes that were being tracked
func (inst *IndependentInstance) ResetChanges() {

	if inst.ChangeTracker != nil {
		inst.ChangeTracker.ResetChanges()
	}

	//todo: can we reuse this to avoid gc
	inst.ChangeTracker = NewInstanceChangeTracker()
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
			inst.ChangeTracker.trackWorkItem(&WorkItemQueueChange{ChgType: CtDel, ID: workItem.ID, WorkItem: workItem})

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
	inst.ChangeTracker.trackWorkItem(&WorkItemQueueChange{ChgType: CtAdd, ID: workItem.ID, WorkItem: workItem})
}

// execTask executes the specified Work Item of the Flow Instance
func (inst *IndependentInstance) execTask(behavior model.TaskBehavior, taskInst *TaskInst) {

	defer func() {
		if r := recover(); r != nil {

			err := fmt.Errorf("unhandled Error executing task '%s' : %v", taskInst.task.ID(), r)
			inst.logger.Error(err)

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
	case model.EvalRepeat:
		taskInst.SetStatus(model.TaskStatusReady)
		//task needs to iterate or retry
		inst.scheduleEval(taskInst)
	}
}

// handleTaskDone handles the completion of a task in the Flow Instance
func (inst *IndependentInstance) handleTaskDone(taskBehavior model.TaskBehavior, taskInst *TaskInst) {

	notifyFlow := false
	var taskEntries []*model.TaskEntry
	var err error

	if taskInst.Status() == model.TaskStatusSkipped {
		notifyFlow, taskEntries = taskBehavior.Skip(taskInst)

	} else {
		notifyFlow, taskEntries, err = taskBehavior.Done(taskInst)
	}

	containerInst := taskInst.flowInst

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

			// spawned from task instance
			host, ok := containerInst.host.(*TaskInst)

			if ok {
				//if the flow failed, set the error
				for name, value := range containerInst.returnData {
					//todo review how we should handle an error encountered here
					host.SetOutput(name, value)
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
			delete(inst.subFlows, containerInst.subFlowId)
		}

	} else {
		// not done, so enter tasks specified by the Done behavior call
		err := inst.enterTasks(containerInst, taskEntries)
		if err != nil {
			//todo review how we should handle an error encountered here
			log.RootLogger().Errorf("encountered error when entering tasks: %v", err)
		}
	}

	// task is done, so we can release it
	containerInst.releaseTask(task)
}

// handleTaskError handles the completion of a task in the Flow Instance
func (inst *IndependentInstance) handleTaskError(taskBehavior model.TaskBehavior, taskInst *TaskInst, err error) {

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

func (inst *IndependentInstance) startInstance(toStart *Instance) bool {

	toStart.SetStatus(model.FlowStatusActive)

	//if pi.Attrs == nil {
	//	pi.Attrs = make(map[string]*data.Attribute)
	//}
	//
	//for _, attr := range startAttrs {
	//	pi.Attrs[attr.Name()] = attr
	//}

	//logger.Infof("FlowInstance Flow: %v", pi.FlowModel)

	//need input mappings

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

func (inst *IndependentInstance) enterTasks(activeInst *Instance, taskEntries []*model.TaskEntry) error {

	for _, taskEntry := range taskEntries {

		//logger.Debugf("EnterTask - TaskEntry: %v", taskEntry)
		taskToEnterBehavior := inst.flowModel.GetTaskBehavior(taskEntry.Task.TypeID())

		enterTaskData, _ := activeInst.FindOrCreateTaskData(taskEntry.Task)

		enterResult := taskToEnterBehavior.Enter(enterTaskData)

		if enterResult == model.EnterEval {
			err := applySettingsMapper(enterTaskData)
			if err != nil {
				return err
			}
			inst.scheduleEval(enterTaskData)
		} else if enterResult == model.EnterSkip {
			//todo optimize skip, just keep skipping and don't schedule eval
			inst.scheduleEval(enterTaskData)
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
	workItem.SubFlowID = taskInst.flowInst.subFlowId

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
func (inst *IndependentInstance) Restart(id string, manager *flowsupport.FlowManager) error {
	inst.id = id
	var err error
	inst.flowDef, err = manager.GetFlow(inst.flowURI)

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

	return nil
}

func (inst *IndependentInstance) init(flowInst *Instance) {

	for _, v := range flowInst.taskInsts {
		v.flowInst = flowInst
		v.task = flowInst.flowDef.GetTask(v.taskID)
	}

	for _, v := range flowInst.linkInsts {
		v.flowInst = flowInst
		v.link = flowInst.flowDef.GetLink(v.linkID)
	}
}

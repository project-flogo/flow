package instance

import (
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/state"
	"github.com/project-flogo/flow/state/change"
)

var defaultChgTracker = &NoopChangeTracker{}
var chgTrackerFactory = &SimpleChangeTrackerFactory{}
var chgTrackingEnabled = false
var stateMode state.RecordingMode

type ChangeTracker interface {
	// FlowCreated is called to track a when a flow is created
	FlowCreated(flow *IndependentInstance)
	// FlowDone is called to track a when a flow complete
	FlowDone(flow *IndependentInstance)
	// SetStatus is called to track a status change on an instance
	SetStatus(subflowId int, status model.FlowStatus)
	// AttrChange is called to track when Attribute changes
	AttrChange(subflowId int, name string, value interface{})
	// SubflowCreated is called to track a when a subflow is created
	SubflowCreated(subflow *Instance)
	// SubflowDone is called to track a when a subflow complete
	SubflowDone(subflow *Instance)
	// WorkItemAdded records when an item is added to the WorkQueue
	WorkItemAdded(wi *WorkItem)
	// WorkItemRemoved records when an item is removed from the WorkQueue
	WorkItemRemoved(wi *WorkItem)
	// TaskAdded records when a Task is added
	TaskAdded(taskInst *TaskInst)
	// TaskUpdated records when a Task is updated
	TaskUpdated(taskInst *TaskInst)
	// TaskRemoved records when a Task is removed
	TaskRemoved(subflowId int, taskId string)
	// LinkAdded records a Link is added
	LinkAdded(linkInst *LinkInst)
	// LinkUpdated records a Link is updated
	LinkUpdated(linkInst *LinkInst)
	// LinkRemoved records when a Link is removed
	LinkRemoved(subflowId int, linkId int)
	// ExtractStep extracts the step object and resets the tracker
	ExtractStep(reset bool) *state.Step
}

func NewInstanceChangeTracker(flowId string) ChangeTracker {
	if chgTrackingEnabled {
		return chgTrackerFactory.NewChangeTracker(flowId, stateMode)
	}
	return defaultChgTracker
}

func EnableChangeTracking(enable bool, mode state.RecordingMode) {
	chgTrackingEnabled = enable
	stateMode = mode
}

type NoopChangeTracker struct {
}

func (nct *NoopChangeTracker) FlowCreated(flow *IndependentInstance) {
}

func (nct *NoopChangeTracker) FlowDone(flow *IndependentInstance) {
}

func (nct *NoopChangeTracker) SetStatus(subflowId int, status model.FlowStatus) {
}

func (nct *NoopChangeTracker) AttrChange(subflowId int, name string, value interface{}) {
}

func (nct *NoopChangeTracker) SubflowCreated(subflow *Instance) {
}

func (nct *NoopChangeTracker) SubflowDone(subflow *Instance) {
}

func (nct *NoopChangeTracker) WorkItemAdded(wi *WorkItem) {
}

func (nct *NoopChangeTracker) WorkItemRemoved(wi *WorkItem) {
}

func (nct *NoopChangeTracker) TaskAdded(taskInst *TaskInst) {
}

func (nct *NoopChangeTracker) TaskUpdated(taskInst *TaskInst) {
}

func (nct *NoopChangeTracker) TaskRemoved(subflowId int, taskId string) {
}

func (nct *NoopChangeTracker) LinkAdded(linkInst *LinkInst) {
}

func (nct *NoopChangeTracker) LinkUpdated(linkInst *LinkInst) {
}

func (nct *NoopChangeTracker) LinkRemoved(subflowId int, linkId int) {
}

func (nct *NoopChangeTracker) ExtractStep(reset bool) *state.Step {
	return nil
}

type SimpleChangeTrackerFactory struct {
}

func (sf *SimpleChangeTrackerFactory) NewChangeTracker(flowId string, mode state.RecordingMode) ChangeTracker {

	ct := &SimpleChangeTracker{flowId: flowId, mode: mode}
	ct.currentStep = &state.Step{
		FlowId:      flowId,
		FlowChanges: make(map[int]*change.Flow),
	}

	return ct
}

type SimpleChangeTracker struct {
	flowId      string
	mode        state.RecordingMode
	stepCtr     int
	currentStep *state.Step
}

func (sct *SimpleChangeTracker) SetStatus(subflowId int, status model.FlowStatus) {

	fc, exists := sct.currentStep.FlowChanges[subflowId]
	if !exists {
		fc = &change.Flow{}
		sct.currentStep.FlowChanges[subflowId] = fc
	}

	fc.Status = int(status)
}

func (sct *SimpleChangeTracker) AttrChange(subflowId int, name string, value interface{}) {
	fc, exists := sct.currentStep.FlowChanges[subflowId]
	if !exists {
		fc = &change.Flow{}
		sct.currentStep.FlowChanges[subflowId] = fc
	}

	if fc.Attrs == nil {
		fc.Attrs = make(map[string]interface{})
	}

	fc.Attrs[name] = value
}

func (sct *SimpleChangeTracker) FlowCreated(flow *IndependentInstance) {
	fc := &change.Flow{
		NewFlow: true,
		FlowURI: flow.flowURI,
		Status:  int(flow.status),
	}
	sct.currentStep.FlowChanges[0] = fc
}

func (sct *SimpleChangeTracker) FlowDone(flow *IndependentInstance) {
	// Save return for debugger mode only
	if sct.mode == state.RecordingModeDebugger {
		flowC := sct.currentStep.FlowChanges[flow.subflowId]
		if flowC != nil {
			flowC.FlowURI = flow.flowURI
			flowC.Status = int(flow.status)
			flowC.ReturnData, _ = flow.GetReturnData()
		} else {
			fc := &change.Flow{
				NewFlow: false,
				FlowURI: flow.flowURI,
				Status:  int(flow.status),
			}
			fc.ReturnData, _ = flow.GetReturnData()
			sct.currentStep.FlowChanges[flow.subflowId] = fc
		}
	}
}

func (sct *SimpleChangeTracker) SubflowCreated(subflow *Instance) {

	host := subflow.host.(*TaskInst)

	fc := &change.Flow{
		NewFlow:   true,
		FlowURI:   subflow.flowURI,
		SubflowId: subflow.subflowId,
		TaskId:    host.taskID,
		Status:    int(subflow.status),
	}
	sct.currentStep.FlowChanges[subflow.subflowId] = fc
}

func (sct *SimpleChangeTracker) SubflowDone(subflow *Instance) {
	// Save return for debugger mode only
	if sct.mode == state.RecordingModeDebugger {
		if sct.currentStep.FlowChanges != nil {
			fc := sct.currentStep.FlowChanges[subflow.subflowId]
			if fc != nil {
				fc.FlowURI = subflow.flowURI
				fc.Status = int(subflow.status)
				fc.ReturnData, _ = subflow.GetReturnData()
			} else {
				fc := &change.Flow{
					NewFlow: false,
					FlowURI: subflow.flowURI,
					Status:  int(subflow.status),
				}
				fc.ReturnData, _ = subflow.GetReturnData()
				sct.currentStep.FlowChanges[subflow.subflowId] = fc
			}
		}
	}
}

func (sct *SimpleChangeTracker) WorkItemAdded(wi *WorkItem) {
	qc := getQueueChange(sct.currentStep, wi.ID)
	qc.TaskId = wi.TaskID
}

func (sct *SimpleChangeTracker) WorkItemRemoved(wi *WorkItem) {
	qc := getQueueChange(sct.currentStep, wi.ID)
	qc.TaskId = wi.TaskID
	qc.ChgType = change.Delete
}

func (sct *SimpleChangeTracker) TaskAdded(taskInst *TaskInst) {
	task := getTaskChange(sct.currentStep, taskInst.flowInst.subflowId, taskInst.taskID)
	task.Status = int(taskInst.status)
}

func (sct *SimpleChangeTracker) TaskUpdated(taskInst *TaskInst) {
	task := getTaskChange(sct.currentStep, taskInst.flowInst.subflowId, taskInst.taskID)
	task.ChgType = change.Update
	task.Status = int(taskInst.status)
	// Store input for debugger mode
	if sct.mode == state.RecordingModeDebugger {
		task.Input = taskInst.inputs
	}
}

func (sct *SimpleChangeTracker) TaskRemoved(subflowId int, taskId string) {
	task := getTaskChange(sct.currentStep, subflowId, taskId)
	task.ChgType = change.Delete
}

func (sct *SimpleChangeTracker) LinkAdded(linkInst *LinkInst) {
	link := getLinkChange(sct.currentStep, linkInst.flowInst.subflowId, linkInst.id)
	link.Status = int(linkInst.status)
	link.From = linkInst.link.FromTask().ID()
	link.To = linkInst.link.ToTask().ID()
}

func (sct *SimpleChangeTracker) LinkUpdated(linkInst *LinkInst) {
	link := getLinkChange(sct.currentStep, linkInst.flowInst.subflowId, linkInst.id)
	link.ChgType = change.Update
	link.Status = int(linkInst.status)
}

func (sct *SimpleChangeTracker) LinkRemoved(subflowId int, linkId int) {
	link := getLinkChange(sct.currentStep, subflowId, linkId)
	link.ChgType = change.Delete
}

func (sct *SimpleChangeTracker) ExtractStep(reset bool) *state.Step {

	step := sct.currentStep

	if reset {
		sct.stepCtr++
		sct.currentStep = &state.Step{
			Id:           sct.stepCtr,
			FlowId:       sct.flowId,
			FlowChanges:  make(map[int]*change.Flow),
			QueueChanges: make(map[int]*change.Queue),
		}
	}

	return step
}

func getQueueChange(step *state.Step, workItemId int) *change.Queue {

	if step.QueueChanges == nil {
		step.QueueChanges = make(map[int]*change.Queue, 2)
	}

	wc, exists := step.QueueChanges[workItemId]
	if !exists {
		wc = &change.Queue{}
		step.QueueChanges[workItemId] = wc
	}

	return wc
}

func getTaskChange(step *state.Step, subflowId int, taskId string) *change.Task {

	fc, exists := step.FlowChanges[subflowId]
	if !exists {
		fc = &change.Flow{TaskId: taskId}
		step.FlowChanges[subflowId] = fc
	}

	if fc.Tasks == nil {
		fc.Tasks = make(map[string]*change.Task, 2)
	}

	tc, exists := fc.Tasks[taskId]
	if !exists {
		tc = &change.Task{}
		fc.Tasks[taskId] = tc
	}

	return tc
}

func getLinkChange(step *state.Step, subflowId int, linkId int) *change.Link {

	fc, exists := step.FlowChanges[subflowId]
	if !exists {
		fc = &change.Flow{}
		step.FlowChanges[subflowId] = fc
	}

	if fc.Links == nil {
		fc.Links = make(map[int]*change.Link, 2)
	}

	lc, exists := fc.Links[linkId]
	if !exists {
		lc = &change.Link{}
		fc.Links[linkId] = lc
	}

	return lc
}

package instance

import (
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/flow/model"
)

type ChangeTracker interface {
	// SetStatus is called to track a status change on an instance
	SetStatus(subFlowId int, status model.FlowStatus)
	// AttrChange is called to track when Attribute changes
	AttrChange(subFlowId int, name string, value interface{})
	// SubFlowCreated is called to track a when a subflow is created
	SubFlowCreated(subFlow *Instance)
	// WorkItemAdded records when an item is added to the WorkQueue
	WorkItemAdded(wi *WorkItem)
	// WorkItemRemoved records when an item is removed from the WorkQueue
	WorkItemRemoved(wi *WorkItem)
	// TaskAdded records when a Task is added
	TaskAdded(subFlowId int, taskInst *TaskInst)
	// TaskUpdated records when a Task is updated
	TaskUpdated(subFlowId int, taskInst *TaskInst)
	// TaskRemoved records when a Task is removed
	TaskRemoved(subFlowId int, taskId string)
	// LinkAdded records a Link is added
	LinkAdded(subFlowId int, linkInst *LinkInst)
	// LinkUpdated records a Link is updated
	LinkUpdated(subFlowId int, linkInst *LinkInst)
	// LinkRemoved records when a Link is removed
	LinkRemoved(subFlowId int, linkId int)
	// ResetChanges is used to reset any tracking data stored on instance objects
	ResetChanges()
}

// ChgType denotes the type of change for an object in an instance
type ChgType int

const (
	// CtAdd denotes an addition
	CtAdd ChgType = 1
	// CtUpd denotes an update
	CtUpd ChgType = 2
	// CtDel denotes an deletion
	CtDel ChgType = 3
)

// WorkItemQueueChange represents a change in the WorkItem Queue
type WorkItemQueueChange struct {
	ChgType  ChgType
	ID       int
	WorkItem *WorkItem
}

// TaskInstChange represents a change to a TaskInst
type TaskInstChange struct {
	ChgType  ChgType
	ID       string
	TaskInst *TaskInst
}

// LinkInstChange represents a change to a LinkInst
type LinkInstChange struct {
	ChgType  ChgType
	ID       int
	LinkInst *LinkInst
}

// InstanceChange represents a change to the instance
type InstanceChange struct {
	SubFlowID   int
	Status      model.FlowStatus
	AttrChanges []*AttributeChange
	tiChanges   map[string]*TaskInstChange
	liChanges   map[int]*LinkInstChange
	SubFlowChg  *SubFlowChange

	//State int
}

// InstanceChange represents a change to the instance
type SubFlowChange struct {
	SubFlowID int
	TaskID    string
	ChgType   ChgType
}

// AttributeChange represents a change to an Attribute
type AttributeChange struct {
	SubFlowID int
	ChgType   ChgType
	Attribute *data.Attribute
}

// InstanceChangeTracker is used to track all changes to an instance
type InstanceChangeTracker struct {
	wiqChanges  map[int]*WorkItemQueueChange
	instChanges map[int]*InstanceChange //at most 2
}

// NewInstanceChangeTracker creates an InstanceChangeTracker
func NewInstanceChangeTracker() *InstanceChangeTracker {
	return &InstanceChangeTracker{instChanges: make(map[int]*InstanceChange)}
}

func (ict *InstanceChangeTracker) getInstChange(flowId int) *InstanceChange {

	change, exists := ict.instChanges[flowId]
	if !exists {
		change = &InstanceChange{}
		ict.instChanges[flowId] = change
	}

	return change
}

// SetStatus is called to track a status change on an instance
func (ict *InstanceChangeTracker) SetStatus(subFlowId int, status model.FlowStatus) {
	ic := ict.getInstChange(subFlowId)
	ic.Status = status
}

// AttrChange is called to track when Attribute changes
func (ict *InstanceChangeTracker) AttrChange(subFlowId int, name string, value interface{}) {

	attribute := data.NewAttribute(name, data.TypeAny, value)

	ic := ict.getInstChange(subFlowId)

	var attrChange AttributeChange
	attrChange.ChgType = CtUpd
	attrChange.Attribute = attribute
	ic.AttrChanges = append(ic.AttrChanges, &attrChange)
}

// SubFlowCreated is called to track a when a subflow is created
func (ict *InstanceChangeTracker) SubFlowCreated(subFlow *Instance) {

	taskInst := subFlow.host.(*TaskInst)
	ic := ict.getInstChange(taskInst.flowInst.subFlowId)

	var change SubFlowChange
	change.ChgType = CtAdd
	change.SubFlowID = subFlow.subFlowId
	change.TaskID = taskInst.task.ID()

	ic.SubFlowChg = &change
}

// WorkItemAdded records when an item is added to the WorkQueue
func (ict *InstanceChangeTracker) WorkItemAdded(wi *WorkItem) {

	wiChange := &WorkItemQueueChange{ChgType: CtAdd, ID: wi.ID, WorkItem: wi}
	if ict.wiqChanges == nil {
		ict.wiqChanges = make(map[int]*WorkItemQueueChange)
	}
	ict.wiqChanges[wiChange.ID] = wiChange
}

// WorkItemRemoved records when an item is removed from the WorkQueue
func (ict *InstanceChangeTracker) WorkItemRemoved(wi *WorkItem) {

	wiChange := &WorkItemQueueChange{ChgType: CtDel, ID: wi.ID, WorkItem: wi}
	if ict.wiqChanges == nil {
		ict.wiqChanges = make(map[int]*WorkItemQueueChange)
	}
	ict.wiqChanges[wiChange.ID] = wiChange
}

// TaskAdded records when a Task is added
func (ict *InstanceChangeTracker) TaskAdded(subFlowId int, taskInst *TaskInst) {

	tdChange := &TaskInstChange{ChgType: CtAdd, ID: taskInst.task.ID(), TaskInst: taskInst}

	ic := ict.getInstChange(subFlowId)

	if ic.tiChanges == nil {
		ic.tiChanges = make(map[string]*TaskInstChange)
	}

	ic.tiChanges[tdChange.ID] = tdChange
}

// TaskUpdated records when a Task is updated
func (ict *InstanceChangeTracker) TaskUpdated(subFlowId int, taskInst *TaskInst) {

	tdChange := &TaskInstChange{ChgType: CtUpd, ID: taskInst.task.ID(), TaskInst: taskInst}

	ic := ict.getInstChange(subFlowId)

	if ic.tiChanges == nil {
		ic.tiChanges = make(map[string]*TaskInstChange)
	}

	ic.tiChanges[tdChange.ID] = tdChange
}

// TaskRemoved records when a Task is removed
func (ict *InstanceChangeTracker) TaskRemoved(subFlowId int, taskId string) {

	tdChange := &TaskInstChange{ChgType: CtDel, ID: taskId}

	ic := ict.getInstChange(subFlowId)

	if ic.tiChanges == nil {
		ic.tiChanges = make(map[string]*TaskInstChange)
	}

	ic.tiChanges[tdChange.ID] = tdChange
}

// LinkAdded records a Link is added
func (ict *InstanceChangeTracker) LinkAdded(subFlowId int, linkInst *LinkInst) {

	ldChange := &LinkInstChange{ChgType: CtAdd, ID: linkInst.link.ID(), LinkInst: linkInst}

	ic := ict.getInstChange(subFlowId)

	if ic.liChanges == nil {
		ic.liChanges = make(map[int]*LinkInstChange)
	}

	ic.liChanges[ldChange.ID] = ldChange
}

// LinkUpdated records a Link is updated
func (ict *InstanceChangeTracker) LinkUpdated(subFlowId int, linkInst *LinkInst) {

	ldChange := &LinkInstChange{ChgType: CtUpd, ID: linkInst.link.ID(), LinkInst: linkInst}

	ic := ict.getInstChange(subFlowId)

	if ic.liChanges == nil {
		ic.liChanges = make(map[int]*LinkInstChange)
	}
	ic.liChanges[ldChange.ID] = ldChange
}

// LinkRemoved records when a Link is removed
func (ict *InstanceChangeTracker) LinkRemoved(subFlowId int, linkId int) {

	ldChange := &LinkInstChange{ChgType: CtDel, ID: linkId}

	ic := ict.getInstChange(subFlowId)

	if ic.liChanges == nil {
		ic.liChanges = make(map[int]*LinkInstChange)
	}
	ic.liChanges[ldChange.ID] = ldChange
}

// ResetChanges is used to reset any tracking data stored on instance objects
func (ict *InstanceChangeTracker) ResetChanges() {

	//// reset TaskInst objects
	//if ict.tdChanges != nil {
	//	for _, v := range ict.tdChanges {
	//		if v.TaskInst != nil {
	//			//v.TaskInst.ResetChanges()
	//		}
	//	}
	//}
	//
	//// reset LinkInst objects
	//if ict.ldChanges != nil {
	//	for _, v := range ict.ldChanges {
	//		if v.LinkInst != nil {
	//			//v.LinkInst.ResetChanges()
	//		}
	//	}
	//}
}

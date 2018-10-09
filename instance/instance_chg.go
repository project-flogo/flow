package instance

import (
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/flow/model"
)

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

	State int
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

// SetStatus is called to track a state change on an instance
func (ict *InstanceChangeTracker) SetState(subFlowId int, state int) {

	ic := ict.getInstChange(subFlowId)
	ic.State = state
}

// SetStatus is called to track a status change on an instance
func (ict *InstanceChangeTracker) SetStatus(subFlowId int, status model.FlowStatus) {
	ic := ict.getInstChange(subFlowId)
	ic.Status = status
}

// AttrChange is called to track a status change of an Attribute
func (ict *InstanceChangeTracker) AttrChange(subFlowId int, chgType ChgType, attribute *data.Attribute) {

	ic := ict.getInstChange(subFlowId)

	var attrChange AttributeChange
	attrChange.ChgType = chgType

	attrChange.Attribute = attribute
	ic.AttrChanges = append(ic.AttrChanges, &attrChange)
}

// AttrChange is called to track a status change of an Attribute
func (ict *InstanceChangeTracker) SubFlowChange(parentFlowId int, chgType ChgType, subFlowId int, taskID string) {

	ic := ict.getInstChange(parentFlowId)

	var change SubFlowChange
	change.ChgType = chgType
	change.SubFlowID = subFlowId
	change.TaskID = taskID

	ic.SubFlowChg = &change
}

// trackWorkItem records a WorkItem Queue change
func (ict *InstanceChangeTracker) trackWorkItem(wiChange *WorkItemQueueChange) {

	if ict.wiqChanges == nil {
		ict.wiqChanges = make(map[int]*WorkItemQueueChange)
	}
	ict.wiqChanges[wiChange.ID] = wiChange
}

// trackTaskData records a TaskInst change
func (ict *InstanceChangeTracker) trackTaskData(subFlowId int, tdChange *TaskInstChange) {

	ic := ict.getInstChange(subFlowId)

	if ic.tiChanges == nil {
		ic.tiChanges = make(map[string]*TaskInstChange)
	}

	ic.tiChanges[tdChange.ID] = tdChange
}

// trackLinkData records a LinkInst change
func (ict *InstanceChangeTracker) trackLinkData(subFlowId int, ldChange *LinkInstChange) {

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

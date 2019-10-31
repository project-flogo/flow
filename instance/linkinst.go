package instance

import (
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/model"
)

// LinkInst represents data associated with an instance of a Link
type LinkInst struct {
	flowInst *Instance
	link     *definition.Link
	status   model.LinkStatus

	//changes int
	id      int //needed for serialization
}

// NewLinkInst creates a LinkInst for the specified link in the specified task
// environment
func NewLinkInst(inst *Instance, link *definition.Link) *LinkInst {
	var linkInst LinkInst

	linkInst.flowInst = inst
	linkInst.link = link

	return &linkInst
}

// Status returns the current state indicator for the LinkInst
func (li *LinkInst) Status() model.LinkStatus {
	return li.status
}

// SetStatus sets the current state indicator for the LinkInst
func (li *LinkInst) SetStatus(status model.LinkStatus) {
	li.status = status
	li.flowInst.master.changeTracker.LinkUpdated(li)
	//ld.flowInst.master.ChangeTracker.trackLinkData(ld.flowInst.subFlowId, &LinkInstChange{ChgType: CtUpd, ID: ld.link.ID(), LinkInst: ld})
}

// Link returns the Link associated with ld context
func (li *LinkInst) Link() *definition.Link {
	return li.link
}

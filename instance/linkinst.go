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

	changes int
	linkID  int //needed for serialization
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
func (ld *LinkInst) Status() model.LinkStatus {
	return ld.status
}

// SetStatus sets the current state indicator for the LinkInst
func (ld *LinkInst) SetStatus(status model.LinkStatus) {
	ld.status = status
	ld.flowInst.master.ChangeTracker.trackLinkData(ld.flowInst.subFlowId, &LinkInstChange{ChgType: CtUpd, ID: ld.link.ID(), LinkInst: ld})
}

// Link returns the Link associated with ld context
func (ld *LinkInst) Link() *definition.Link {
	return ld.link
}

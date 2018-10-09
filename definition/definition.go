package definition

import (
	"fmt"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
)

// Definition is the object that describes the definition of
// a flow.  It contains its data (attributes) and
// structure (tasks & links).
type Definition struct {
	name          string
	modelID       string
	explicitReply bool

	attrs map[string]*data.Attribute

	links map[int]*Link
	tasks map[string]*Task

	metadata *metadata.IOMetadata

	errorHandler *ErrorHandler
}

// Name returns the name of the definition
func (d *Definition) Name() string {
	return d.name
}

// ModelID returns the ID of the model the definition uses
func (d *Definition) ModelID() string {
	return d.modelID
}

// Metadata returns IO metadata for the flow
func (d *Definition) Metadata() *metadata.IOMetadata {
	return d.metadata
}

// GetTask returns the task with the specified ID
func (d *Definition) GetTask(taskID string) *Task {
	task := d.tasks[taskID]
	return task
}

// GetLink returns the link with the specified ID
func (d *Definition) GetLink(linkID int) *Link {
	task := d.links[linkID]
	return task
}

func (d *Definition) ExplicitReply() bool {
	return d.explicitReply
}

func (d *Definition) GetErrorHandler() *ErrorHandler {
	return d.errorHandler
}

// GetAttr gets the specified attribute
func (d *Definition) GetAttr(attrName string) (attr *data.Attribute, exists bool) {

	if d.attrs != nil {
		attr, found := d.attrs[attrName]
		if found {
			return attr, true
		}
	}

	return nil, false
}

// GetTask returns the task with the specified ID
func (d *Definition) Tasks() []*Task {

	tasks := make([]*Task, 0, len(d.tasks))
	for _, task := range d.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

func (d *Definition) Links() []*Link {
	links := make([]*Link, 0, len(d.links))
	for _, link := range d.links {
		links = append(links, link)
	}
	return links
}

//// SetLinkExprManager sets the LinkOld Expr Manager for the definition
//func (d *Definition) SetLinkExprManager(mgr LinkExprManager) {
//	// todo revisit
//	d.linkExprMgr = mgr
//}
//
//// GetLinkExprManager gets the LinkOld Expr Manager for the definition
//func (d *Definition) GetLinkExprManager() LinkExprManager {
//	return d.linkExprMgr
//}

type ActivityConfig struct {
	Activity activity.Activity
	//Details     *activity.Details

	settings map[string]*data.Attribute
	//inputAttrs  map[string]*data.Attribute
	//outputAttrs map[string]*data.Attribute

	inputMapper  mapper.Mapper
	outputMapper mapper.Mapper
	Details      *activity.Details
}

func (ac *ActivityConfig) Ref() string {
	return activity.GetRef(ac.Activity)
}

// GetSetting gets the specified setting
func (ac *ActivityConfig) GetSetting(setting string) (attr *data.Attribute, exists bool) {

	if ac.settings != nil {
		attr, found := ac.settings[setting]
		if found {
			return attr, true
		}
	}

	return nil, false
}

//// GetAttr gets the specified input attribute
//func (ac *ActivityConfig) GetInputAttr(attrName string) (attr *data.Attribute, exists bool) {
//
//	if ac.inputAttrs != nil {
//		attr, found := ac.inputAttrs[attrName]
//		if found {
//			return attr, true
//		}
//	}
//
//	return nil, false
//}
//
//// GetOutputAttr gets the specified output attribute
//func (ac *ActivityConfig) GetOutputAttr(attrName string) (attr *data.Attribute, exists bool) {
//
//	if ac.outputAttrs != nil {
//		attr, found := ac.outputAttrs[attrName]
//		if found {
//			return attr, true
//		}
//	}
//
//	return nil, false
//}

// InputMapper returns the InputMapper of the task
func (ac *ActivityConfig) InputMapper() mapper.Mapper {
	return ac.inputMapper
}

// OutputMapper returns the OutputMapper of the task
func (ac *ActivityConfig) OutputMapper() mapper.Mapper {
	return ac.outputMapper
}

//func (ac *ActivityConfig) Ref() string {
//	return ac.Activity.Metadata().ID
//}

// Task is the object that describes the definition of
// a task.  It contains its data (attributes) and its
// nested structure (child tasks & child links).
type Task struct {
	definition *Definition
	id         string
	typeID     string
	name       string

	activityCfg *ActivityConfig

	isScope bool

	settings map[string]interface{}

	//inputs map[string]data.TypedValue
	//inputAttrs  map[string]*data.Attribute
	//outputAttrs map[string]*data.Attribute

	//inputMapper  mapper.Mapper
	//outputMapper mapper.Mapper

	toLinks   []*Link
	fromLinks []*Link
}

// ID gets the id of the task
func (task *Task) ID() string {
	return task.id
}

// Name gets the name of the task
func (task *Task) Name() string {
	return task.name
}

// TypeID gets the id of the task type
func (task *Task) TypeID() string {
	return task.typeID
}

//// GetAttr gets the specified attribute
//// DEPRECATED
//func (task *Task) GetAttr(attrName string) (attr *data.Attribute, exists bool) {
//
//	if task.inputAttrs != nil {
//		attr, found := task.inputAttrs[attrName]
//		if found {
//			return attr, true
//		}
//	}
//
//	return nil, false
//}
//
//// GetAttr gets the specified input attribute
//func (task *Task) GetInputAttr(attrName string) (attr *data.Attribute, exists bool) {
//
//	if task.inputAttrs != nil {
//		attr, found := task.inputAttrs[attrName]
//		if found {
//			return attr, true
//		}
//	}
//
//	return nil, false
//}
//
//// GetOutputAttr gets the specified output attribute
//func (task *Task) GetOutputAttr(attrName string) (attr *data.Attribute, exists bool) {
//
//	if task.outputAttrs != nil {
//		attr, found := task.outputAttrs[attrName]
//		if found {
//			return attr, true
//		}
//	}
//
//	return nil, false
//}

func (task *Task) ActivityConfig() *ActivityConfig {
	return task.activityCfg
}

func (task *Task) GetSetting(attrName string) (value interface{}, exists bool) {
	value, exists = task.settings[attrName]
	return value, exists
}

// ToLinks returns the predecessor links of the task
func (task *Task) ToLinks() []*Link {
	return task.toLinks
}

// FromLinks returns the successor links of the task
func (task *Task) FromLinks() []*Link {
	return task.fromLinks
}

func (task *Task) String() string {
	return fmt.Sprintf("Task[%s] '%s'", task.id, task.name)
}

// IsScope returns flag indicating if the Task is a scope task (a container of attributes)
func (task *Task) IsScope() bool {
	return task.isScope
}

////////////////////////////////////////////////////////////////////////////
// Link

// LinkType is an enum for possible Link Types
type LinkType int

const (
	// LtDependency denotes an normal dependency link
	LtDependency LinkType = 0

	// LtExpression denotes a link with an expression
	LtExpression LinkType = 1 //expr language on the model or def?

	// LtLabel denotes 'label' link
	LtLabel LinkType = 2

	// LtError denotes an error link
	LtError LinkType = 3
)

// LinkOld is the object that describes the definition of
// a link.
type Link struct {
	definition *Definition
	id         int
	name       string
	fromTask   *Task
	toTask     *Task
	linkType   LinkType
	value      string //expression or label

	expr expression.Expr
}

// ID gets the id of the link
func (link *Link) ID() int {
	return link.id
}

// Type gets the link type
func (link *Link) Type() LinkType {
	return link.linkType
}

// TypedValue gets the "value" of the link
func (link *Link) Value() string {
	return link.value
}

// FromTask returns the task the link is coming from
func (link *Link) FromTask() *Task {
	return link.fromTask
}

// ToTask returns the task the link is going to
func (link *Link) ToTask() *Task {
	return link.toTask
}

// Expr returns the expr associated with the link, nil if there is none
func (link *Link) Expr() expression.Expr {
	return link.expr
}

func (link *Link) String() string {
	return fmt.Sprintf("Link[%d]:'%s' - [from:%s, to:%s]", link.id, link.name, link.fromTask.id, link.toTask.id)
}

type ErrorHandler struct {
	links map[int]*Link
	tasks map[string]*Task
}

func (eh *ErrorHandler) Tasks() []*Task {

	tasks := make([]*Task, 0, len(eh.tasks))
	for _, task := range eh.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

package definition

import (
	"encoding/json"
	"fmt"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/data/schema"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
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

func (d *Definition) Cleanup() error {
	for id, task := range d.tasks {
		if !activity.IsSingleton(task.activityCfg.Activity) {
			if needsDisposal, ok := task.activityCfg.Activity.(support.NeedsCleanup); ok {
				err := needsDisposal.Cleanup()
				if err != nil {
					log.RootLogger().Warnf("Error disposing activity '%s' : ", id, err)
				}
			}
		}
	}

	return nil
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

type ActivityConfig struct {
	Activity activity.Activity
	Logger   log.Logger

	settings map[string]interface{}

	inputMapper  mapper.Mapper
	outputMapper mapper.Mapper

	inputSchemas  map[string]schema.Schema
	outputSchemas map[string]schema.Schema

	Details *activity.Details

	outputs  map[string]interface{}
	IsLegacy bool
}

func (ac *ActivityConfig) GetInputSchema(name string) schema.Schema {
	if ac.inputSchemas != nil {
		return ac.inputSchemas[name]
	}

	return nil
}

//Deprecated
func (ac *ActivityConfig) GetOutput(name string) interface{} {
	if ac.outputs != nil {
		return ac.outputs[name]
	}
	return nil
}

func (ac *ActivityConfig) GetOutputSchema(name string) schema.Schema {
	if ac.outputSchemas != nil {
		return ac.outputSchemas[name]
	}

	return nil
}

func (ac *ActivityConfig) Ref() string {
	return activity.GetRef(ac.Activity)
}

// GetSetting gets the specified setting
func (ac *ActivityConfig) GetSetting(setting string) (val interface{}, exists bool) {

	if ac.settings != nil {
		val, found := ac.settings[setting]
		if found {
			return val, true
		}
	}

	return nil, false
}

// InputMapper returns the InputMapper of the task
func (ac *ActivityConfig) InputMapper() mapper.Mapper {
	return ac.inputMapper
}

// OutputMapper returns the OutputMapper of the task
func (ac *ActivityConfig) OutputMapper() mapper.Mapper {
	return ac.outputMapper
}

type LoopConfig struct {
	condition      expression.Expr
	accumulate     bool
	delay          int
	iterateOn      interface{}
	accApplyOutput bool
}

func (l *LoopConfig) Accumulate() bool {
	return l.accumulate
}

func (l *LoopConfig) ApplyOutputOnAccumulate() bool {
	return l.accApplyOutput
}

func (l *LoopConfig) Condition() expression.Expr {
	return l.condition
}

func (l *LoopConfig) GetIterateOn() interface{} {
	return l.iterateOn
}

func (l *LoopConfig) Delay() int {
	return l.delay
}

type RetryOnErrConfig struct {
	count    int
	interval int
}

func (r *RetryOnErrConfig) Count() int {
	return r.count
}

func (r *RetryOnErrConfig) Interval() int {
	return r.interval
}

func (r *RetryOnErrConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Count    int `md:"count"`
		Interval int `md:"interval"`
	}{
		Count:    r.count,
		Interval: r.interval,
	})
}

// UnmarshalJSON implements json.Unmarshaler.UnmarshalJSON
func (r *RetryOnErrConfig) UnmarshalJSON(data []byte) error {

	ser := &struct {
		Count    int `md:"count"`
		Interval int `md:"interval"`
	}{}

	if err := json.Unmarshal(data, ser); err != nil {
		return err
	}
	r.count = ser.Count
	r.interval = ser.Interval
	return nil
}

// Task is the object that describes the definition of
// a task.  It contains its data (attributes) and its
// nested structure (child tasks & child links).
type Task struct {
	definition *Definition
	id         string
	typeID     string
	name       string

	activityCfg *ActivityConfig
	isScope     bool

	settingsMapper mapper.Mapper

	loopCfg          *LoopConfig
	retryOnErrConfig *RetryOnErrConfig

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

func (task *Task) ActivityConfig() *ActivityConfig {
	return task.activityCfg
}

// SettingsMapper returns the SettingsMapper of the task
func (task *Task) SettingsMapper() mapper.Mapper {
	return task.settingsMapper
}

func (task *Task) RetryOnErrConfig() *RetryOnErrConfig {
	return task.retryOnErrConfig
}

func (task *Task) LoopConfig() *LoopConfig {
	return task.loopCfg
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

	// LtExprOtherwise denotes an expression otherwise link
	LtExprOtherwise = 4
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

func (eh *ErrorHandler) GetTask(taskID string) *Task {
	return eh.tasks[taskID]
}

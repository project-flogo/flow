package model

// FlowModel defines the execution Model for a Flow.  It contains the
// execution behaviors for Flows and Tasks.
type FlowModel struct {
	name                string
	flowBehavior        FlowBehavior
	defaultTaskBehavior TaskBehavior
	taskBehaviors       map[string]TaskBehavior
}

// New creates a new FlowModel from the specified Behaviors
func New(name string) *FlowModel {

	var flowModel FlowModel
	flowModel.name = name
	flowModel.taskBehaviors = make(map[string]TaskBehavior)

	return &flowModel
}

// Name returns the name of the FlowModel
func (fm *FlowModel) Name() string {
	return fm.name
}

// RegisterFlowBehavior registers the specified FlowBehavior with the Model
func (fm *FlowModel) RegisterFlowBehavior(flowBehavior FlowBehavior) {

	fm.flowBehavior = flowBehavior
}

// GetFlowBehavior returns FlowBehavior of the FlowModel
func (fm *FlowModel) GetFlowBehavior() FlowBehavior {
	return fm.flowBehavior
}

// RegisterDefaultTaskBehavior registers the default TaskBehavior for the Model
func (fm *FlowModel) GetDefaultTaskBehavior() TaskBehavior {
	return fm.defaultTaskBehavior
}

func (fm *FlowModel) RegisterDefaultTaskBehavior(id string, taskBehavior TaskBehavior) {

	fm.RegisterTaskBehavior(id, taskBehavior)
	fm.defaultTaskBehavior = taskBehavior
}

// RegisterTaskBehavior registers the specified TaskBehavior with the Model
func (fm *FlowModel) RegisterTaskBehavior(id string, taskBehavior TaskBehavior) {
	fm.taskBehaviors[id] = taskBehavior
}

func (fm *FlowModel) IsValidTaskType(taskType string) bool {

	if taskType == "" && fm.defaultTaskBehavior != nil {
		return true
	}

	_, exists := fm.taskBehaviors[taskType]
	return exists
}

// GetTaskBehavior returns TaskBehavior with the specified ID in he FlowModel
func (fm *FlowModel) GetTaskBehavior(id string) TaskBehavior {

	if id == "" {
		return fm.defaultTaskBehavior
	}

	return fm.taskBehaviors[id]
}

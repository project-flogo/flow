package instance

import (
	"time"

	coreevent "github.com/project-flogo/core/engine/event"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/support/event"
)

type taskEvent struct {
	time                                time.Time
	id                                  string
	err                                 error
	taskIn, taskOut                     map[string]interface{}
	status                              event.Status
	name, typeId, flowName, flowId, ref string
}

// Returns activity ref
func (te *taskEvent) ActivityRef() string {
	return te.ref
}

// Returns flow name
func (te *taskEvent) FlowName() string {
	return te.flowName
}

// Returns flow ID
func (te *taskEvent) FlowID() string {
	return te.flowId
}

// Returns task name
func (te *taskEvent) TaskName() string {
	return te.name
}

// Returns task instance id
func (te *taskEvent) TaskInstanceId() string {
	return te.id
}

// Returns task type
func (te *taskEvent) TaskType() string {
	return te.typeId
}

// Returns task status
func (te *taskEvent) TaskStatus() event.Status {
	return te.status
}

// Returns event time
func (te *taskEvent) Time() time.Time {
	return te.time
}

// Returns activity input data
func (te *taskEvent) TaskInput() map[string]interface{} {
	return te.taskIn
}

// Returns output data for completed activity
func (te *taskEvent) TaskOutput() map[string]interface{} {
	return te.taskOut
}

// Returns error for failed task
func (te *taskEvent) TaskError() error {
	return te.err
}

func convertTaskStatus(code model.TaskStatus) event.Status {
	switch code {
	case model.TaskStatusNotStarted:
		return event.CREATED
	case model.TaskStatusEntered:
		return event.SCHEDULED
	case model.TaskStatusSkipped:
		return event.SKIPPED
	case model.TaskStatusReady:
		return event.STARTED
	case model.TaskStatusFailed:
		return event.FAILED
	case model.TaskStatusDone:
		return event.COMPLETED
	case model.TaskStatusWaiting:
		return event.WAITING
	}
	return event.UNKNOWN
}

func postTaskEvent(taskInstance *TaskInst) {

	if coreevent.HasListener(event.TaskEventType) {
		te := &taskEvent{}
		te.time = time.Now()
		te.name = taskInstance.Task().Name()
		te.status = convertTaskStatus(taskInstance.Status())
		te.flowName = taskInstance.flowInst.Name()
		te.flowId = taskInstance.flowInst.ID()
		te.typeId = taskInstance.Task().TypeID()
		te.id = taskInstance.id

		if taskInstance.HasActivity() {
			te.ref = taskInstance.Task().ActivityConfig().Ref()
		}

		if te.status == event.FAILED {
			te.err = taskInstance.returnError
		}

		te.taskIn = make(map[string]interface{})
		te.taskOut = make(map[string]interface{})

		// Add working data
		//wData := taskInstance.workingData
		//if wData != nil && len(wData) > 0 {
		//	for name, attVal := range wData {
		//		te.taskIn[name] = attVal
		//	}
		//}

		// Add activity input/output
		// TODO optimize this computation for given instance
		if taskInstance.HasActivity() {

			actConfig := taskInstance.Task().ActivityConfig()

			if actConfig != nil && actConfig.Activity != nil && actConfig.Activity.Metadata() != nil {
				metadata := actConfig.Activity.Metadata()
				if metadata.Input != nil && len(metadata.Input) > 0 && taskInstance.Task().IsScope() {

					for name, attVal := range actConfig.Activity.Metadata().Input {
						te.taskIn[name] = attVal.Value()
						if taskInstance.inputs != nil {
							scopedValue, ok := taskInstance.inputs[name]
							if ok {
								te.taskIn[name] = scopedValue
							}
						}

					}
				}

				if te.status == event.COMPLETED && metadata.Output != nil && len(metadata.Output) > 0 && taskInstance.Task().IsScope() {
					for name, attVal := range actConfig.Activity.Metadata().Output {
						te.taskOut[name] = attVal.Value()
						if taskInstance.outputs != nil {
							scopedValue, ok := taskInstance.outputs[name]
							if ok {
								te.taskOut[name] = scopedValue
							}
						}

					}
				}

				if metadata.IOMetadata != nil {
					if metadata.IOMetadata.Input != nil {
						for name, attVal := range actConfig.Activity.Metadata().Input {
							te.taskIn[name] = attVal.Value()
							if taskInstance.inputs != nil {
								scopedValue, ok := taskInstance.inputs[name]
								if ok {
									te.taskIn[name] = scopedValue
								}
							}
						}
					}

					if te.status == event.COMPLETED && metadata.IOMetadata.Output != nil {
						for name, attVal := range actConfig.Activity.Metadata().Output {
							te.taskOut[name] = attVal.Value()
							if taskInstance.outputs != nil {
								scopedValue, ok := taskInstance.outputs[name]
								if ok {
									te.taskOut[name] = scopedValue
								}
							}
						}
					}

				}

			}
		}
		coreevent.Post(event.TaskEventType, te)
	}

}

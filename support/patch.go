package support

import (
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/mapper"
)

// Patch contains a set of task patches for a Flow Patch, this
// can be used to override the default data and mappings of a Flow
type Patch struct {
	TaskPatches []*TaskPatch `json:"tasks"` //put in mapper object

	taskPatchMap map[string]*TaskPatch
}

// Init initializes the FlowPatch, usually called after deserialization
func (pp *Patch) Init() {

	numAttrs := len(pp.TaskPatches)
	if numAttrs > 0 {

		pp.taskPatchMap = make(map[string]*TaskPatch, numAttrs)

		for _, patch := range pp.TaskPatches {
			pp.taskPatchMap[patch.ID] = patch
		}
	}
}

// GetPatch returns the Task Patch for the specified task (referred to by ID)
func (pp *Patch) GetPatch(taskID string) *TaskPatch {
	return pp.taskPatchMap[taskID]
}

// GetInputMapper returns the InputMapper for the specified task (referred to by ID)
func (pp *Patch) GetInputMapper(taskID string) mapper.Mapper {
	taskPatch, exists := pp.taskPatchMap[taskID]

	if exists {
		return taskPatch.InputMapper()
	}

	return nil
}

// GetOutputMapper returns the OutputMapper for the specified task (referred to by ID)
func (pp *Patch) GetOutputMapper(taskID string) mapper.Mapper {
	taskPatch, exists := pp.taskPatchMap[taskID]

	if exists {
		return taskPatch.OutputMapper()
	}

	return nil
}

// TaskPatch contains patching information for a Task, such has attributes,
// input mappings, output mappings.  This is used to override the corresponding
// settings for a Task in the Process
type TaskPatch struct {
	ID         string                 `json:"id"`
	Attributes []*data.Attribute      `json:"attributes"`
	Input      map[string]interface{} `json:"input"`
	Output     map[string]interface{} `json:"output"`

	Attrs        map[string]*data.Attribute
	inputMapper  mapper.Mapper
	outputMapper mapper.Mapper
}

// InputMapper returns the overriding InputMapper
func (tp *TaskPatch) InputMapper() mapper.Mapper {
	return tp.inputMapper
}

// OutputMapper returns the overriding OutputMapper
func (tp *TaskPatch) OutputMapper() mapper.Mapper {
	return tp.outputMapper
}

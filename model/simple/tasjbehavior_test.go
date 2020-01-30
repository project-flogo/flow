package simple

import (
	"github.com/magiconair/properties/assert"
	"github.com/project-flogo/flow/model"
	"testing"
)

func TestSortTaskEntries(t *testing.T) {
	var taskEntries []*model.TaskEntry
	taskEntry := &model.TaskEntry{EnterCode: 3}
	taskEntry2 := &model.TaskEntry{EnterCode: 0}
	taskEntry3 := &model.TaskEntry{EnterCode: 0}
	taskEntries = append(taskEntries, taskEntry, taskEntry2, taskEntry3)

	SortTaskEntries(taskEntries)
	assert.Equal(t, taskEntries[0], taskEntry2)
	assert.Equal(t, taskEntries[1], taskEntry3)
	assert.Equal(t, taskEntries[2], taskEntry)

	var taskEntries2 []*model.TaskEntry
	taskEntry4 := &model.TaskEntry{EnterCode: 3}
	taskEntry5 := &model.TaskEntry{EnterCode: 0}
	taskEntry6 := &model.TaskEntry{EnterCode: 0}
	taskEntry7 := &model.TaskEntry{EnterCode: 3}
	taskEntry8 := &model.TaskEntry{EnterCode: 0}

	taskEntries2 = append(taskEntries2, taskEntry4, taskEntry5, taskEntry6, taskEntry7, taskEntry8)

	SortTaskEntries(taskEntries2)

	assert.Equal(t, taskEntries2[0], taskEntry5)
	assert.Equal(t, taskEntries2[1], taskEntry6)
	assert.Equal(t, taskEntries2[2], taskEntry8)
	assert.Equal(t, taskEntries2[3], taskEntry4)
	assert.Equal(t, taskEntries2[4], taskEntry7)
}

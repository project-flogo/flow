package state

import (
	"encoding/json"
	"fmt"
	"github.com/project-flogo/flow/state/change"
	"github.com/stretchr/testify/assert"
	"testing"
)

func CreateTestSteps1() []*Step {

	s0 := &Step{
		Id:     0,
		FlowId: "blah",
		FlowChanges: map[int]*change.Flow{0: {
			NewFlow: true,
			FlowURI: "blahFlowDef",
			Status:  10,
			Attrs:   map[string]interface{}{"input": "testVal"},
			Tasks:   map[string]*change.Task{"t1": {Status: 10}},
		}},
		QueueChanges: map[int]*change.Queue{0: {TaskId: "t1"}},
	}
	s1 := &Step{
		Id:     1,
		FlowId: "blah",
		FlowChanges:map[int]*change.Flow{0: {
			Attrs:   map[string]interface{}{"t1.out": "out1"},
			Tasks:   map[string]*change.Task{"t1": {Status: 50},"t2": {Status: 10}},
			Links:   map[int]*change.Link{0: {Status: 50}},
		}},
		QueueChanges: map[int]*change.Queue{0: {ChgType: change.Delete}, 1: {TaskId: "t2"}},
	}
	s2 := &Step{
		Id:     2,
		FlowId: "blah",
		FlowChanges:map[int]*change.Flow{0: {
			Attrs:   map[string]interface{}{"t2.out": "out2"},
			Tasks:   map[string]*change.Task{"t2": {Status: 50},"t3": {Status: 10}},
			Links:   map[int]*change.Link{1: {Status: 50}},
		}},
		QueueChanges: map[int]*change.Queue{1: {ChgType: change.Delete}, 2: {TaskId: "t3"}},
	}
	s3 := &Step{
		Id:     3,
		FlowId: "blah",
		FlowChanges:map[int]*change.Flow{0: {
			Status:50,
			Attrs:   map[string]interface{}{"t3.out": "out3"},
			Tasks:   map[string]*change.Task{"t3": {Status: 50}},
			Links:   map[int]*change.Link{1: {Status: 50}},
		}},
		QueueChanges: map[int]*change.Queue{2: {ChgType: change.Delete}},
	}

	steps := []*Step{s0, s1, s2, s3}

	return steps
}

func TestStepsToSnapshot(t *testing.T) {

	steps := CreateTestSteps1()
	snapshot := StepsToSnapshot("blah", steps)

	jSnapshot, err := json.MarshalIndent(&snapshot, "", "  ")
	assert.Nil(t, err)

	if err == nil {
		fmt.Printf("snapshot: %s\n", string(jSnapshot))
	}

	snapshot = StepsToSnapshot("blah", steps[:3])

	jSnapshot, err = json.MarshalIndent(&snapshot, "", "  ")
	assert.Nil(t, err)

	if err == nil {
		fmt.Printf("snapshot: %s\n", string(jSnapshot))
	}
}

package simple

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	_ = activity.LegacyRegister("log", NewLogActivity())
}

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

func TestTaskBehaviour(t *testing.T) {

	testContext := &TestTaskContext{}

	taskBehaviour := &TaskBehavior{}

	assert.NotNil(t, taskBehaviour.Enter(testContext))
	result, err := taskBehaviour.Eval(testContext)
	assert.Nil(t, err)
	assert.Equal(t, model.EvalDone, result)
	// KNOWN PRE-EXISTING FAILURE (unrelated to FLOGO-18450 / the core bump): the
	// `assert.True(t, skip)` below fails. This was previously hidden because model/simple
	// could not compile against the old pinned core (missing trace.TagDefs); once core is
	// bumped and the package compiles, the latent bug is exposed.
	//
	// Why it fails:
	//   1. TaskBehavior.Skip in taskbehavior.go declares a NAMED return `propagateSkip bool`
	//      that SHADOWS the package-level `var propagateSkip = support.GetPropagateSkip()`.
	//      The function never assigns the named return, so Skip always returns
	//      propagateSkip = false regardless of FLOGO_TASK_PROPAGATE_SKIP.
	//   2. Even without the shadowing, the package-level var is read ONCE at init, so this
	//      os.Setenv at test time cannot change it.
	//
	// How to fix (separate change, needs review as it alters default skip propagation):
	//   - In taskbehavior.go Skip(), drop the shadowing named return (or assign it), e.g.
	//     `return true, nil, propagateSkip` should return the package/env value, not the
	//     zero-valued named return.
	//   - Make this test drive the value deterministically (reset/inject the cached var or
	//     read the env inside Skip) instead of relying on a post-init os.Setenv.
	os.Setenv("FLOGO_TASK_PROPAGATE_SKIP", "true")
	notify, tasks, skip := taskBehaviour.Skip(testContext)
	assert.True(t, notify)
	assert.Nil(t, tasks)
	assert.True(t, skip)
}

type TestTaskContext struct {
	mock.Mock
	status model.TaskStatus
	data   map[string]interface{}
}

func (t *TestTaskContext) Status() model.TaskStatus {
	return t.status
}

// SetStatus sets the state of the Task instance
func (t *TestTaskContext) SetStatus(status model.TaskStatus) {
	t.status = status
}

// Task returns the Task associated with this context
func (t *TestTaskContext) Task() *definition.Task {
	return getDef().Tasks()[0]
}

// GetFromLinkInstances returns the instances of predecessor Links of the current task.
func (t *TestTaskContext) GetFromLinkInstances() []model.LinkInstance {
	return nil
}

// GetToLinkInstances returns the instances of successor Links of the current task.
func (t *TestTaskContext) GetToLinkInstances() []model.LinkInstance {
	return nil
}

// EvalLink evaluates the specified link
func (t *TestTaskContext) EvalLink(link *definition.Link) (bool, error) {
	return true, nil
}

// EvalActivity evaluates the Activity associated with the Task
func (t *TestTaskContext) EvalActivity() (done bool, err error) {
	return true, nil
}

// PostActivity does post evaluation of the Activity associated with the Task
func (t *TestTaskContext) PostEvalActivity() (done bool, err error) {
	return true, nil
}

func (t *TestTaskContext) GetSetting(name string) (value interface{}, exists bool) {
	return "test", true
}

func (t *TestTaskContext) SetWorkingData(key string, value interface{}) {

}

func (t *TestTaskContext) GetWorkingData(key string) (interface{}, bool) {
	_, ok := t.data[key]
	if !ok {
		return nil, false
	}
	return "test", true
}

func (t *TestTaskContext) GetGoContext() context.Context {
	return nil
}

func (t *TestTaskContext) FlowLogger() log.Logger {
	return log.RootLogger()
}

const defTestJSON = `
{
	"id":"",
  "name": "Demo Flow",
   "metadata": {
      "input":[
        { "name":"petInfo", "type":"string","value":"blahPet" }
      ]
    },
	"tasks": [
	{
	  "id":"LogStart",
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "Find Pet Flow Started!"
        }
      }
	},
	{
	  "id": "LogResult",
	  "name": "Log Results",
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "=$.petInfo"
        }
      }
    }
    ],
    "links": [
      { "id": 1, "name": "", "from": "LogStart", "to": "LogResult"  }
    ]
  }`

func getDef() *definition.Definition {

	defRep := &definition.DefinitionRep{}
	json.Unmarshal([]byte(defTestJSON), defRep)

	def, _ := definition.NewDefinition(defRep)
	return def
}

type LogActivity struct {
	metadata *activity.Metadata
}

// NewActivity creates a new AppActivity
func NewLogActivity() activity.Activity {
	md := &activity.Metadata{IOMetadata: &metadata.IOMetadata{Input: map[string]data.TypedValue{"message": data.NewTypedValue(data.TypeString, "")}}}
	return &LogActivity{metadata: md}
}

// Metadata returns the activity's metadata
func (a *LogActivity) Metadata() *activity.Metadata {
	return a.metadata
}

// Eval implements api.Activity.Eval - Logs the Message
func (a *LogActivity) Eval(context activity.Context) (done bool, err error) {

	//mv := context.GetInput(ivMessage)
	message, _ := context.GetInput("message").(string)

	fmt.Println("Message :", message)
	return true, nil
}

package subflow

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/TIBCOSoftware/flogo-contrib/action/flow"
	"github.com/TIBCOSoftware/flogo-lib/app/resource"
	"github.com/TIBCOSoftware/flogo-lib/core/action"
	"github.com/TIBCOSoftware/flogo-lib/core/activity"
	"github.com/TIBCOSoftware/flogo-lib/core/data"
	"github.com/TIBCOSoftware/flogo-lib/engine/runner"
	"github.com/stretchr/testify/assert"
)

var activityMetadata *activity.Metadata

func getActivityMetadata() *activity.Metadata {

	if activityMetadata == nil {
		jsonMetadataBytes, err := ioutil.ReadFile("activity.json")
		if err != nil {
			panic("No Json Metadata found for activity.json path")
		}

		activityMetadata = activity.NewMetadata(string(jsonMetadataBytes))
	}

	return activityMetadata
}

var jsonFlowRes1 = `{
"id":"flow:flow1",
"data":
{
  "attributes": [],
  "tasks": [
    {
      "id": "runFlow",
      "activity": {
        "ref": "github.com/TIBCOSoftware/flogo-contrib/activity/subflow",
        "settings": {
          "flowURI": "res://flow/flow2"
        },
        "mappings": {
          "input": [
            { "type": 2, "value": "test", "mapTo": "in" }
          ],
          "output": [
          ]
        }
      }
    }
  ],
  "links": [
  ]
}
}
`

var jsonFlowRes2 = `{
"id":"flow:flow2",
"data":
{
  "metadata": {
    "input": [
      {
        "name": "in",
        "type": "string"
      }
    ],
    "output": [
      {
        "name": "value",
        "type": "string"
      }
    ]
  },
  "attributes": [],
  "tasks": [
    {
      "id": "log",
      "activity": {
        "ref": "log",
        "mappings": {
          "input": [
            { "type": 1, "value": "$flow.in", "mapTo": "message" }
          ],
          "output": [
          ]
        }
      }
    }
  ],
  "links": [
  ]
}
}`

var jsonFlow1 = `{
  "attributes": [],
  "tasks": [
    {
      "id": "runFlow",
      "activity": {
        "ref": "github.com/TIBCOSoftware/flogo-contrib/activity/subflow",
        "settings": {
          "flowURI": "res://flow:flow2"
        },
        "mappings": {
          "input": [
            { "type": 2, "value": "test", "mapTo": "in" }
          ],
          "output": [
          ]
        }
      }
    }
  ],
  "links": [
  ]
}
`

var jsonFlow2 = `{
  "metadata": {
    "input": [
      {
        "name": "in",
        "type": "string"
      }
    ],
    "output": [
      {
        "name": "value",
        "type": "string"
      }
    ]
  },
  "attributes": [],
  "tasks": [
    {
      "id": "log",
      "activity": {
        "ref": "log",
        "mappings": {
          "input": [
            { "type": 1, "value": "$flow.in", "mapTo": "message" }
          ],
          "output": [
          ]
        }
      }
    }
  ],
  "links": [
  ]
}
`

func TestCreate(t *testing.T) {

	act := NewActivity(getActivityMetadata())

	if act == nil {
		t.Error("Activity Not Created")
		t.Fail()
		return
	}
}

func TestDynamicIO(t *testing.T) {

	act := NewActivity(getActivityMetadata())

	if act == nil {
		t.Error("Activity Not Created")
		t.Fail()
		return
	}

	assert.True(t, act.Metadata().DynamicIO)

	_, ok := act.(activity.DynamicIO)

	assert.True(t, ok)
}

func TestSubFlow(t *testing.T) {

	act := NewActivity(getActivityMetadata())

	if act == nil {
		t.Error("Activity Not Created")
		t.Fail()
		return
	}

	activity.Register(act)
	activity.Register(NewLogActivity())

	f := action.GetFactory(flow.FLOW_REF)
	af := f.(*flow.ActionFactory)
	af.Init()

	rConfig1 := &resource.Config{ID: "flow:flow1", Data: []byte(jsonFlow1)}
	rConfig2 := &resource.Config{ID: "flow:flow2", Data: []byte(jsonFlow2)}
	err := resource.Load(rConfig1)
	assert.Nil(t, err)
	err = resource.Load(rConfig2)
	assert.Nil(t, err)
	flowAction, err := f.New(&action.Config{Data: []byte(`{"flowURI":"res://flow:flow1"}`)})

	assert.Nil(t, err)
	assert.NotNil(t, flowAction)

	dr := runner.NewDirect()
	results, err := dr.Execute(context.Background(), flowAction, nil)
	assert.Nil(t, err)
	assert.NotNil(t, results)
}

//DUMMY TEST ACTIVITIES

type LogActivity struct {
	metadata *activity.Metadata
}

// NewActivity creates a new AppActivity
func NewLogActivity() activity.Activity {
	metadata := &activity.Metadata{ID: "log"}
	input := map[string]*data.Attribute{
		"message": data.NewZeroAttribute("message", data.TypeString),
	}
	metadata.Input = input
	return &LogActivity{metadata: metadata}
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

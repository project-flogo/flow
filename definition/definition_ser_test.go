package definition

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/stretchr/testify/assert"
)

func init() {
	activity.Register(NewLogActivity())
}

const defJSON = `
{
	"id":"DemoFlow",
    "name": "Demo Flow",
    "model": "simple",
    "attributes": [
      { "name": "petInfo", "type": "string", "value": "blahPet" }
    ],
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
           "message" : "REST results"
        },
        "mappings" : {
          "input": [
		    { "type": 1, "value": "petInfo", "result": "message" }
          ]
        }
      }
    }
    ],
    "links": [
      { "id": 1, "type": "1",  "name": "", "from": "LogStart", "to": "LogResult"  }
    ]
  }
`

const oldDefJSON = `
{
    "type": 1,
    "name": "Demo Flow",
    "model": "simple",
    "attributes": [
      { "name": "petInfo", "type": "string", "value": "blahPet" }
    ],
    "rootTask": {
      "id": 1,
      "type": 1,
      "name": "root",
	  "activityRef": "",
      "tasks": [
        {
          "id": 2,
          "type": 1,
          "activityRef": "log",
          "name": "Log Start",
          "attributes": [
            { "type": "string", "name": "message", "value": "Find Pet Flow Started!"}
          ]
        },
        {
          "id": 3,
          "type": 1,
          "activityRef": "log",
          "name": "Log Results",
          "attributes": [
            { "type": "string", "name": "message", "value": "REST results" }
          ],
          "inputMappings": [
            { "type": 1, "value": "petInfo", "result": "message" }
          ]
        }
      ],
      "links": [
        { "id": 1, "type": 1,  "name": "", "from": 2, "to": 3  }
      ]
    }
  }
`

func TestDeserialize(t *testing.T) {

	defRep := &DefinitionRep{}

	err := json.Unmarshal([]byte(defJSON), defRep)
	assert.Nil(t, err)

	def, err := NewDefinition(defRep)
	assert.Nil(t, err)

	fmt.Printf("Definition: %v", def)
}

func TestDeserializeOld(t *testing.T) {

	defRep := &DefinitionRep{}

	err := json.Unmarshal([]byte(oldDefJSON), defRep)
	assert.Nil(t, err)

	def, err := NewDefinition(defRep)
	assert.Nil(t, err)

	fmt.Printf("Definition: %v", def)
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

package instance

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
	"github.com/stretchr/testify/assert"
)
func init() {
	_ = activity.LegacyRegister("log", NewLogActivity())
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
  }
`

func TestIndependentAction(t *testing.T) {
	defRep := &definition.DefinitionRep{}

	err := json.Unmarshal([]byte(defTestJSON), defRep)
	assert.Nil(t, err)

	def, err := definition.NewDefinition(defRep)
	assert.Nil(t, err)
	assert.NotNil(t, def)
	//fmt.Println(def.ModelID())

	ind , err := NewIndependentInstance("test","", def, nil,  log.RootLogger())
	assert.Nil(t, err)
	assert.NotNil(t, ind)

	assert.NotNil(t,ind.ID())
	assert.NotNil(t, ind.Snapshot())

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

func getDef() *definition.Definition {

	defRep := &definition.DefinitionRep{}
	json.Unmarshal([]byte(defTestJSON), defRep)

	def, _ := definition.NewDefinition(defRep)

	return def
}
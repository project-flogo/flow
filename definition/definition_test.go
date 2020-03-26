package definition

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

const defRetryJSON = `
{
	"id":"DemoFlow",
  "name": "Demo Flow",
   "metadata": {
      "input":[
        { "name":"petInfo", "type":"string","value":"blahPet" }
      ]
    },
	"tasks": [
	{
	  "id":"LogStart",
		"settings" :{
      "loopConfig" :{
        "accumulate": true,
  			"delay":2
      },
			"retryOnError" : {
        "count": 1,
        "interval": 100
      }
		},
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

func TestRetry(t *testing.T) {
	defRep := &DefinitionRep{}

	err := json.Unmarshal([]byte(defRetryJSON), defRep)
	assert.Nil(t, err)

	def, err := NewDefinition(defRep)
	assert.Nil(t, err)
	assert.NotNil(t, def)
	task := def.GetTask("LogStart")
	assert.NotNil(t, task)

	assert.Equal(t, 2, task.LoopConfig().Delay())

	assert.Equal(t, 100, task.RetryOnErrorInterval())
}

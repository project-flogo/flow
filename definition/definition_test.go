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

const defBackJSON = `
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
      		"doWhile": {
				"condition": "1",
				"delay": 0
				},
			"accumulate": false
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

func TestNewDefinition(t *testing.T) {
	defRep := &DefinitionRep{}

	err := json.Unmarshal([]byte(defJSON), defRep)
	assert.Nil(t, err)

	def, err := NewDefinition(defRep)

	assert.Nil(t, err)
	assert.Equal(t, "DemoFlow", def.ModelID())
	assert.Equal(t, "Demo Flow", def.Name())
	assert.NotNil(t, def.Metadata())
	assert.Equal(t, false, def.ExplicitReply())
	assert.Nil(t, def.GetErrorHandler())

	assert.NotNil(t, def.GetTask("LogStart"))
	assert.NotNil(t, def.GetLink(0))

	assert.Equal(t, 2, len(def.Tasks()))
	assert.Equal(t, 1, len(def.Links()))

	task := def.Tasks()[0]

	assert.NotNil(t, task.ActivityConfig())
	assert.NotNil(t, task.ID())
	assert.NotNil(t, task.IsScope())
	assert.NotNil(t, task.Name())
	assert.NotNil(t, task.String())
}
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

	oldDefRep := &DefinitionRep{}
	err = json.Unmarshal([]byte(defBackJSON), oldDefRep)
	assert.Nil(t, err)

	def, err = NewDefinition(oldDefRep)
	assert.Nil(t, err)
	assert.NotNil(t, def)
	task = def.GetTask("LogStart")
	assert.NotNil(t, task)



}

package definition

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/expression"
	_ "github.com/project-flogo/core/data/expression/script"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
	flowUtil "github.com/project-flogo/flow/util"
	"github.com/stretchr/testify/assert"
)

func init() {
	_ = activity.LegacyRegister("log", NewLogActivity())

	flowUtil.RegisterModelValidator("test", &dummyModelValidator{})
	exprFactory := expression.NewFactory(GetDataResolver())
	mapperFactory := mapper.NewFactory(GetDataResolver())

	SetMapperFactory(mapperFactory)
	SetExprFactory(exprFactory)
}

type dummyModelValidator struct {
}

func (d *dummyModelValidator) IsValidTaskType(taskType string) bool {
	return true
}

const defWithLoop = `
{
  "name": "Test Flow",
  "model":"test",
  "metadata": {
    "input":[
      { "name":"aValue", "type":"string", "value":"foo" }
    ]
  },
  "tasks": [
    {
      "id":"LogLoop",
       "type": "doWhile",
       "settings" :{
         "condition" : "=$iteration[index] < 5", 
         "accumulate": true,
         "delay": 5,
         "retryOnError" : {
           "count": 1,
           "interval": 100
         }
       },
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "A Log Statement"
        }
      }
	},
	{
	  "id": "LogSingle",
	  "name": "Log Single",
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "=$flow[aValue]"
        }
      }
    }
    ],
    "links": [
      { "id": 1, "from": "LogLoop", "to": "LogSingle"  }
    ]
  }
`

const defWithLoopDeprecated1 = `
{
  "name": "Test Flow",
  "model":"test",
  "metadata": {
    "input":[
      { "name":"aValue", "type":"string", "value":"foo" }
    ]
  },
  "tasks": [
    {
      "id":"LogLoop",
       "type": "doWhile",
       "settings" :{
         "loopConfig": {
           "condition" : "=$iteration[index] < 5", 
           "accumulate": true,
           "delay": 5
         },
         "retryOnError" : {
           "count": 1,
           "interval": 100
         }
       },
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "A Log Statement"
        }
      }
	},
	{
	  "id": "LogSingle",
	  "name": "Log Single",
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "=$flow[aValue]"
        }
      }
    }
    ],
    "links": [
      { "id": 1, "from": "LogLoop", "to": "LogSingle"  }
    ]
  }
`

const defWithLoopDeprecated2 = `
{
  "name": "Test Flow",
  "model":"test",
  "metadata": {
    "input":[
      { "name":"aValue", "type":"string", "value":"foo" }
    ]
  },
  "tasks": [
    {
      "id":"LogLoop",
       "type": "doWhile",
       "settings" :{
         "doWhile": {
           "condition" : "=$iteration[index] < 5", 
           "delay": 5
         },
         "accumulate": true,
         "retryOnError" : {
           "count": 1,
           "interval": 100
         }
       },
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "A Log Statement"
        }
      }
	},
	{
	  "id": "LogSingle",
	  "name": "Log Single",
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "=$flow[aValue]"
        }
      }
    }
    ],
    "links": [
      { "id": 1, "from": "LogLoop", "to": "LogSingle"  }
    ]
  }
`

func TestDefWithLoop(t *testing.T) {
	validate(t, defWithLoop)
}
func TestDefWithLoopDeprecated1(t *testing.T) {
	validate(t, defWithLoopDeprecated1)
}

func TestDefWithLoopDeprecated2(t *testing.T) {
	validate(t, defWithLoopDeprecated2)
}

func validate(t *testing.T, defJson string) {
	defRep := &DefinitionRep{}

	err := json.Unmarshal([]byte(defJson), defRep)
	assert.Nil(t, err)

	def, err := NewDefinition(defRep)

	assert.Nil(t, err)
	assert.Equal(t, "Test Flow", def.Name())
	assert.NotNil(t, def.Metadata())
	assert.Equal(t, false, def.ExplicitReply())
	assert.Nil(t, def.GetErrorHandler())

	assert.NotNil(t, def.GetLink(0))
	assert.Equal(t, 2, len(def.Tasks()))
	assert.Equal(t, 1, len(def.Links()))

	task := def.GetTask("LogLoop")
	assert.NotNil(t, task)
	assert.Equal(t, "doWhile", task.TypeID())
	assert.NotNil(t, task.SettingsMapper())

	loopCfg := task.LoopConfig()
	assert.NotNil(t, loopCfg)
	assert.NotNil(t, loopCfg.Condition())
	assert.Nil(t, loopCfg.GetIterateOn())
	assert.Equal(t, 5, loopCfg.Delay())
	assert.True(t, loopCfg.Accumulate())
	assert.True(t, loopCfg.ApplyOutputOnAccumulate())

	retryOnErrCfg := task.RetryOnErrConfig()
	assert.NotNil(t, retryOnErrCfg)
	assert.Equal(t, 1, retryOnErrCfg.Count())
	assert.Equal(t, 100, retryOnErrCfg.Interval())

	ac := task.ActivityConfig()
	assert.NotNil(t, ac)
	assert.Equal(t, "github.com/project-flogo/flow/definition", ac.Ref())

	task = def.GetTask("LogSingle")

	assert.NotNil(t, task)
	ac = task.ActivityConfig()
	assert.NotNil(t, ac)
	assert.Equal(t, "github.com/project-flogo/flow/definition", ac.Ref())

	assert.Nil(t, task.LoopConfig())
	assert.Nil(t, task.RetryOnErrConfig())
}

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

const defErrJSON = `
{
  "type": 1,
  "name": "Demo Flow",
  "model": "simple",
 "metadata": {
      "input":[
        { "name":"petInfo", "type":"string","value":"blahPet" }
      ]
  },
  "tasks": [
     {
	  "id": "LogResult",
	  "name": "Log Results",
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "message"
        }
      }
    }
  ],
  "links": [ ],
  "errorHandler": {
    "tasks" :[
      {
	  "id": "LogErrorResult",
	  "name": "Log Error",
	  "activity" : {
	    "ref":"log",
        "input" : {
           "message" : "Error Log"
        }
      }
    }
    ],
    "links" : [  ]
  }
}
`

func TestDeserializeOld(t *testing.T) {

	defRep := &DefinitionRep{}

	err := json.Unmarshal([]byte(oldDefJSON), defRep)
	assert.Nil(t, err)

	def, err := NewDefinition(defRep)
	assert.Nil(t, err)
	assert.NotNil(t, def)

}

func TestDeserializeError(t *testing.T) {

	defRep := &DefinitionRep{}

	err := json.Unmarshal([]byte(defErrJSON), defRep)
	assert.Nil(t, err)

	def, err := NewDefinition(defRep)
	assert.Nil(t, err)
	assert.NotNil(t, def)

}

//DUMMY TEST ACTIVITIES

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

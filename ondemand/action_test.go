package ondemand

import (
	"context"
	"encoding/json"
	"testing"

	_ "github.com/project-flogo/contrib/activity/log"
	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/app/resource"

	"github.com/project-flogo/core/engine/runner"
	"github.com/stretchr/testify/assert"
)

const testEventJson = `
{
  "payload": {
    "in1":"in1_value",
    "in2":"in2_value"
  },
  "flogo" : {
      "inputs": { 
        "customerId": "=$.payload.in1",
          "orderId": "=$.payload.in2" 
        }
      ,
      "flow": {
        "metadata" : {
          "input":[
            { "name":"customerId", "type":"string" },
            { "name":"orderId", "type":"string" }
          ],
          "output":[
            { "name":"value", "type":"string" }
          ]
        },
        "tasks": [
          {
            "id": "LogResult",
            "name": "Log Results",
            "activity" : {
              "ref":"github.com/project-flogo/contrib/activity/log",
              "input" : {
                "message" : "=$flow.orderId"
              }
            }
          }
        ]
      }
  }
}`

type testInitCtx struct {
}

func (testInitCtx) ResourceManager() *resource.Manager {
	return nil
}

type Event struct {
	Payload interface{}     `json:"payload"`
	Flogo   json.RawMessage `json:"flogo"`
}

//TestInitNoFlavorError
func TestFlowAction_Run(t *testing.T) {

	var evt Event

	// Unmarshall evt
	if err := json.Unmarshal([]byte(testEventJson), &evt); err != nil {
		assert.Nil(t, err)
		return
	}

	cfg := &action.Config{}

	ff := ActionFactory{}
	ff.Initialize(&testInitCtx{})
	fa, err := ff.New(cfg)
	assert.Nil(t, err)

	flowAction, ok := fa.(action.AsyncAction)
	assert.True(t, ok)

	inputs := make(map[string]interface{}, 2)

	inputs["flowPackage"] = evt.Flogo

	inputs["payload"] = evt.Payload

	r := runner.NewDirect()
	_, err = r.RunAction(context.Background(), flowAction, inputs)

	assert.Nil(t, err)
}

package flow

import (
	"encoding/json"
	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/app/resource"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/engine/runner"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/support"
	"github.com/project-flogo/flow/tester"
	"github.com/stretchr/testify/assert"
	"testing"
)

const FLOW_REF = "github.com/project-flogo/flow"

type testInitCtx struct {
}

func (testInitCtx) NewExprFactory(resolver resolver.CompositeResolver) expr.Factory {
	return nil
}

func (testInitCtx) NewMapperFactory(resolver resolver.CompositeResolver) mapper.MapperFactory {
	return nil
}

func (testInitCtx) ResourceManager() *resource.Manager {
	return nil
}

//TestInitNoFlavorError
func TestInitNoFlavorError(t *testing.T) {

	mockConfig := &action.Config{Id: "myMockConfig", Ref: "github.com/my/mock/ref", Data: []byte(`{}`)}
	f := &ActionFactory{}
	f.Initialize(&testInitCtx{})
	_, err := f.New(mockConfig)
	assert.NotNil(t, err)
}

//TestInitUnCompressedFlowFlavorError
func TestInitUnCompressedFlowFlavorError(t *testing.T) {

	mockFlowData := []byte(`{"flow":{}}`)

	mockConfig := &action.Config{Id: "myMockConfig", Ref: "github.com/my/mock/ref", Data: mockFlowData}

	f := &ActionFactory{}
	f.Initialize(&testInitCtx{})
	_, err := f.New(mockConfig)
	assert.Nil(t, err)
}

//TestInitCompressedFlowFlavorError
func TestInitCompressedFlowFlavorError(t *testing.T) {

	mockFlowData := []byte(`{"flowCompressed":""}`)

	mockConfig := &action.Config{Id: "myMockConfig", Ref: "github.com/my/mock/ref", Data: mockFlowData}

	f := &ActionFactory{}
	f.Initialize(&testInitCtx{})
	_, err := f.New(mockConfig)
	assert.NotNil(t, err)
}

//TestInitURIFlowFlavorError
func TestInitURIFlowFlavorError(t *testing.T) {

	mockFlowData := []byte(`{"flowURI":""}`)

	mockConfig := &action.Config{Id: "myMockConfig", Ref: "github.com/my/mock/ref", Data: mockFlowData}

	f := &ActionFactory{}
	f.Initialize(&testInitCtx{})
	_, err := f.New(mockConfig)
	assert.NotNil(t, err)
}

var testFlowActionCfg = `{
 "id": "flow",
 "ref": "github.com/TIBCOSoftware/flogo-contrib/action/flow",
 "metadata": {
   "input": [],
   "output": []
 },
 "data":{
 "flow": {
   "model": "tibco-simple",
   "type": 1,
   "attributes": [],
   "rootTask": {
     "id": 1,
     "type": 1,
     "activityType": "",
     "ref": "",
     "name": "root",
     "tasks": [
       {
         "id": "log_2",
         "name": "Log Message",
         "description": "Simple Log Activity",
         "type": 1,
         "activityType": "tibco-log",
         "activityRef": "github.com/TIBCOSoftware/flogo-contrib/activity/log",
         "attributes": [
           {
             "name": "message",
             "value": "First log",
             "required": false,
             "type": "string"
           },
           {
             "name": "flowInfo",
             "value": "false",
             "required": false,
             "type": "boolean"
           },
           {
             "name": "addToFlow",
             "value": "true",
             "required": false,
             "type": "boolean"
           }
         ]
       },
       {
         "id": "log_3",
         "name": "Log Message (2)",
         "description": "Simple Log Activity",
         "type": 1,
         "activityType": "tibco-log",
         "activityRef": "github.com/TIBCOSoftware/flogo-contrib/activity/log",
         "attributes": [
           {
             "name": "message",
             "value": "Second log",
             "required": false,
             "type": "string"
           },
           {
             "name": "flowInfo",
             "value": "false",
             "required": false,
             "type": "boolean"
           },
           {
             "name": "addToFlow",
             "value": "true",
             "required": false,
             "type": "boolean"
           }
         ]
       }
     ],
     "links": [
       {
         "id": 1,
         "from": "log_2",
         "to": "log_3",
         "type": 0
       }
     ]
   }
 }
 }
}
`
var testRestartInitialState = `{
 "initialState": {
   "id": "90c3f713bf2b87e4e9a584892039a76b",
   "state": 0,
   "status": 100,
   "attrs": [],
   "flowUri": "flow",
   "workQueue": [
     {
       "id": 2,
       "execType": 10,
       "taskID": "log_2",
       "code": 0
     }
   ],
   "rootTaskEnv": {
     "id": 1,
     "taskId": "1",
     "taskDatas": [
       {
         "state": 20,
         "done": false,
         "attrs": [],
         "taskId": "log_2"
       }
     ],
     "linkDatas": []
   },
   "actionUri": "flow"
 },
 "interceptor": {
   "tasks": [
     {
       "id": "log_2",
       "input": [
         {
           "name": "message",
           "type": "string",
           "value": "test rerun 1"
         },
         {
           "name": "flowInfo",
           "type": "boolean",
           "value": "false"
         },
         {
           "name": "addToFlow",
           "type": "boolean",
           "value": "true"
         }
       ]
     }
   ]
 }
}
`

func TestFlowAction_Run_Restart(t *testing.T) {

	cfg := &action.Config{}
	err := json.Unmarshal([]byte(testFlowActionCfg), cfg)

	if err != nil {
		t.Error(err)
		return
	}

	ff := ActionFactory{}
	ff.Initialize(&testInitCtx{})
	flowAction, err := ff.New(cfg)
	assert.NotNil(t, err)

	req := &RestartRequest{}
	err = json.Unmarshal([]byte(testRestartInitialState), req)

	if err != nil {
		t.Error(err)
		return
	}

	ctx := context.Background()

	if req.Data != nil {

		attrs := make([]*data.Attribute, len(req.Data))

		for k, v := range req.Data {
			attr := data.NewAttribute(k, data.TypeAny, v)
			attrs = append(attrs, attr)
		}

		//ctx = trigger.NewContext(context.Background(), attrs)
		ctx = context.Background()
	}

	execOptions := &instance.ExecOptions{Interceptor: req.Interceptor, Patch: req.Patch}
	ro := &instance.RunOptions{Op: instance.OpRestart, ReturnID: true, FlowURI: req.InitialState.FlowURI(), InitialState: req.InitialState, ExecOptions: execOptions}
	inputs := make(map[string]interface{}, 1)
	//attr := data.NewAttribute("_run_options", data.TypeAny, ro)
	inputs["_run_options"] = ro

	r := runner.NewDirect()
	r.RunAction(ctx, flowAction, inputs)
}

type RestartRequest struct {
	InitialState *instance.IndependentInstance `json:"initialState"`
	Data         map[string]interface{}        `json:"data"`
	Interceptor  *support.Interceptor          `json:"interceptor"`
	Patch        *support.Patch                `json:"patch"`
}

var jsonFlow1 = `{
   "name": "HelloWorld",
   "model": "tibco-simple",
   "type": 1,
   "attributes": [],
   "rootTask": {
     "id": "root",
     "type": 1,
     "activityType": "",
     "ref": "",
     "name": "root",
     "tasks": [
       {
         "id": "counter_1",
         "name": "Number Counter",
         "description": "Simple Global Counter Activity",
         "type": 1,
         "activityRef": "test-counter",
         "attributes": [
           {
             "name": "counterName",
             "value": "number",
             "required": false,
             "type": "string"
           }
         ]
       },
       {
         "id": "log_1",
         "name": "Logger",
         "description": "Simple Log Activity",
         "type": 1,
         "activityRef": "test-log",
         "attributes": [
           {
             "name": "message",
             "value": "hello world orig",
             "required": false,
             "type": "string"
           }
         ]
       }
     ],
     "links": [
       {
         "id": 1,
         "from": "counter_1",
         "to": "log_1",
         "type": 0
       }
     ]
   }
 }
`

var jsonRestartRequest = `{
 "initialState": {
   "id": "4f60c4a3dac609293a2214f4cc6ddec1",
   "state": 0,
   "status": 100,
   "attrs": [
     {
       "name": "_A.counter_1.value",
       "type": "integer",
       "value": 2
     }
   ],
   "flowUri": "res://flow:flow1",
   "workQueue": [
     {
       "id": 3,
       "execType": 10,
       "taskID": "log_1",
       "code": 0
     }
   ],
   "rootTaskEnv": {
     "id": 1,
     "taskId": "root",
     "taskDatas": [
       {
         "state": 20,
         "done": false,
         "attrs": [],
         "taskId": "log_1"
       }
     ],
     "linkDatas": [
       {
         "state": 2,
         "attrs": null,
         "linkId": 1
       }
     ]
   },
   "actionUri": "http://localhost:9090/flows/43"
 },
 "interceptor": {
   "tasks": [
     {
       "id": "log_1",
       "input": [
         {
           "name": "message",
           "type": "string",
           "value": "hello world",
           "required": false
         }
       ]
     }
   ]
 }
}
`

func TestRequestProcessor_RestartFlow(t *testing.T) {

	f := action.GetFactory(FLOW_REF)
	af := f.(*ActionFactory)
	af.Initialize(&testInitCtx{})

	loader := resource.GetLoader("flow")
	rConfig1 := &resource.Config{ID: "flow:flow1", Data: []byte(jsonFlow1)}
	_, err := loader.LoadResource(rConfig1)
	assert.Nil(t, err)

	rp := tester.NewRequestProcessor()

	req := &tester.RestartRequest{}
	err = json.Unmarshal([]byte(jsonRestartRequest), req)
	assert.Nil(t, err)

	var results map[string]interface{}

	results, err = rp.RestartFlow(req)
	assert.Nil(t, err)
	assert.NotNil(t, results)

	//results, err := rp.RestartFlow(req)
}

package tester

import (
	"context"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/engine/runner"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/support"
)

const (
	RefFlow = "github.com/project-flogo/flow"
)

// RequestProcessor processes request objects and invokes the corresponding
// flow Manager methods
type RequestProcessor struct {
	runner action.Runner
	logger log.Logger
}

// NewRequestProcessor creates a new RequestProcessor
func NewRequestProcessor() *RequestProcessor {

	var rp RequestProcessor
	rp.runner = runner.NewDirect()
	//todo what logger should this use?
	rp.logger = log.RootLogger()

	return &rp
}

// StartFlow handles a StartRequest for a FlowInstance.  This will
// generate an ID for the new FlowInstance and queue a StartRequest.
func (rp *RequestProcessor) StartFlow(startRequest *StartRequest) (results map[string]interface{}, err error) {

	logger := rp.logger

	logger.Debugf("Tester starting flow")

	//todo share action, for now add flowUri to settings
	settings := map[string]interface{}{"flowURI":startRequest.FlowURI}

	factory := action.GetFactory(RefFlow)
	act, err := factory.New(&action.Config{Settings:settings})
	if err != nil {
		return nil, err
	}

	var inputs map[string]interface{}

	if len(startRequest.Attrs) > 0 {

		logger.Debugf("Starting with flow attrs: %#v", startRequest.Attrs)

		inputs = make(map[string]interface{}, len(startRequest.Attrs)+1)
		for name, value := range startRequest.Attrs {
			inputs[name] = value
		}
	} else if len(startRequest.Data) > 0 {

		logger.Debugf("Starting with flow attrs: %#v", startRequest.Data)

		inputs = make(map[string]interface{}, len(startRequest.Data)+1)

		for k, v := range startRequest.Data {
			//t, err := data.GetType(v)
			//if err != nil {
			//	t = data.TypeAny
			//}
			//attr, _ := data.NewAttribute(k, t, v)
			inputs[k] = v
		}
	} else {
		inputs = make(map[string]interface{}, 1)
	}

	execOptions := &instance.ExecOptions{Interceptor: startRequest.Interceptor, Patch: startRequest.Patch}
	ro := &instance.RunOptions{Op: instance.OpStart, ReturnID: true, FlowURI: startRequest.FlowURI, ExecOptions: execOptions}
	//attr, _ := data.NewAttribute("_run_options", data.TypeAny, ro)
	inputs["_run_options"] = ro

	return rp.runner.RunAction(context.Background(), act, inputs)
}

// RestartFlow handles a RestartRequest for a FlowInstance.  This will
// generate an ID for the new FlowInstance and queue a RestartRequest.
func (rp *RequestProcessor) RestartFlow(restartRequest *RestartRequest) (results map[string]interface{}, err error) {

	logger := rp.logger

	logger.Debugf("Tester restarting flow")

	//todo share action, for now add flowUri to settings
	settings := map[string]interface{}{"flowURI":restartRequest.InitialState.FlowURI()}

	factory := action.GetFactory(RefFlow)
	act, err := factory.New(&action.Config{Settings:settings})
	if err != nil {
		return nil, err
	}

	inputs := make(map[string]interface{}, len(restartRequest.Data)+1)

	if restartRequest.Data != nil {

		logger.Debugf("Updating flow attrs: %v", restartRequest.Data)

		for k, v := range restartRequest.Data {
			//attr, _ := data.NewAttribute(k, data.TypeAny, v)
			inputs[k] = v
		}
	}

	execOptions := &instance.ExecOptions{Interceptor: restartRequest.Interceptor, Patch: restartRequest.Patch}
	ro := &instance.RunOptions{Op: instance.OpRestart, ReturnID: true, FlowURI: restartRequest.InitialState.FlowURI(), InitialState: restartRequest.InitialState, ExecOptions: execOptions}
	//attr, _ := data.NewAttribute("_run_options", data.TypeAny, ro)
	inputs["_run_options"] = ro

	return rp.runner.RunAction(context.Background(), act, inputs)
}

// ResumeFlow handles a ResumeRequest for a FlowInstance.  This will
// queue a RestartRequest.
func (rp *RequestProcessor) ResumeFlow(resumeRequest *ResumeRequest) (results map[string]interface{}, err error) {

	logger := rp.logger

	logger.Debugf("Tester resuming flow")

	factory := action.GetFactory(RefFlow)
	act, _ := factory.New(&action.Config{})

	inputs := make(map[string]interface{}, len(resumeRequest.Data)+1)

	if resumeRequest.Data != nil {

		logger.Debugf("Updating flow attrs: %v", resumeRequest.Data)

		for k, v := range resumeRequest.Data {
			//attr, _ := data.NewAttribute(k, data.TypeAny, v)
			inputs[k] = v
		}
	}

	execOptions := &instance.ExecOptions{Interceptor: resumeRequest.Interceptor, Patch: resumeRequest.Patch}
	ro := &instance.RunOptions{Op: instance.OpResume, ReturnID: true, FlowURI: resumeRequest.State.FlowURI(), InitialState: resumeRequest.State, ExecOptions: execOptions}
	//attr, _ := data.NewAttribute("_run_options", data.TypeAny, ro)
	//inputs[attr.Name()] = attr
	inputs["_run_options"] = ro
	return rp.runner.RunAction(context.Background(), act, inputs)
}

// StartRequest describes a request for starting a FlowInstance
type StartRequest struct {
	FlowURI     string                 `json:"flowUri"`
	Data        map[string]interface{} `json:"data"`
	Attrs       map[string]interface{} `json:"attrs"`
	Interceptor *support.Interceptor   `json:"interceptor"`
	Patch       *support.Patch         `json:"patch"`
	ReplyTo     string                 `json:"replyTo"`
}

// RestartRequest describes a request for restarting a FlowInstance
// todo: can be merged into StartRequest
type RestartRequest struct {
	InitialState *instance.IndependentInstance `json:"initialState"`
	Data         map[string]interface{}        `json:"data"`
	Interceptor  *support.Interceptor          `json:"interceptor"`
	Patch        *support.Patch                `json:"patch"`
}

// ResumeRequest describes a request for resuming a FlowInstance
//todo: Data for resume request should be directed to waiting task
type ResumeRequest struct {
	State       *instance.IndependentInstance `json:"state"`
	Data        map[string]interface{}        `json:"data"`
	Interceptor *support.Interceptor          `json:"interceptor"`
	Patch       *support.Patch                `json:"patch"`
}

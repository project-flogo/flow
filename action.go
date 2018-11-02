package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/app/resource"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
	_ "github.com/project-flogo/flow/model/simple"
	flowsupport "github.com/project-flogo/flow/support"
	"github.com/project-flogo/flow/tester"
)

const (
	ENV_FLOW_RECORD = "FLOGO_FLOW_RECORD"
)

func init() {
	action.Register(&FlowAction{}, &ActionFactory{})
	resource.RegisterLoader(flowsupport.RESTYPE_FLOW, &flowsupport.FlowLoader{})
}

//DEPRECATED
type ActionData struct {
	// The flow is a URI
	//DEPRECATED
	FlowURI string `json:"flowURI"`
}

var ep ExtensionProvider
var idGenerator *support.Generator
var record bool
var maxStepCount = 1000000
var actionMd = action.ToMetadata(&Settings{})
var logger log.Logger

var flowManager *flowsupport.FlowManager

func SetExtensionProvider(provider ExtensionProvider) {
	ep = provider
}

type ActionFactory struct {
	resManager *resource.Manager
}

func (f *ActionFactory) Initialize(ctx action.InitContext) error {

	f.resManager = ctx.ResourceManager()
	logger = log.ChildLogger(log.RootLogger(), "flow")

	if flowManager != nil {
		return nil
	}

	if ep == nil {
		testerEnabled := os.Getenv(tester.ENV_ENABLED)
		if strings.ToLower(testerEnabled) == "true" {
			ep = tester.NewExtensionProvider()

			sm := support.GetDefaultServiceManager()
			sm.RegisterService(ep.GetFlowTester())
			record = true
		} else {
			ep = NewDefaultExtensionProvider()
			record = recordFlows()
		}
	}

	exprFactory := expression.NewFactory(definition.GetDataResolver())
	mapperFactory := mapper.NewFactory(definition.GetDataResolver())

	definition.SetMapperFactory(mapperFactory)
	definition.SetExprFactory(exprFactory)

	if idGenerator == nil {
		idGenerator, _ = support.NewGenerator()
	}

	model.RegisterDefault(ep.GetDefaultFlowModel())
	flowManager = flowsupport.NewFlowManager(ep.GetFlowProvider())
	resource.RegisterLoader(flowsupport.RESTYPE_FLOW, &flowsupport.FlowLoader{})

	flowsupport.InitDefaultDefLookup(flowManager, ctx.ResourceManager())

	return nil
}

func recordFlows() bool {
	recordFlows := os.Getenv(ENV_FLOW_RECORD)
	if len(recordFlows) == 0 {
		return false
	}
	b, _ := strconv.ParseBool(recordFlows)
	return b
}

func (f *ActionFactory) New(config *action.Config) (action.Action, error) {

	flowAction := &FlowAction{}

	if config.Data != nil {
		return flowAction, nil

		var actionData ActionData
		err := json.Unmarshal(config.Data, &actionData)
		if err != nil {
			return nil, fmt.Errorf("faild to load flow action data '%s' error '%s'", config.Id, err.Error())
		}

		if len(actionData.FlowURI) > 0 {

			flowAction.flowURI = actionData.FlowURI
		}
	} else {
		settings := &Settings{}
		err := metadata.MapToStruct(config.Settings, settings, true)
		if err != nil {
			return nil, err
		}

		flowAction.flowURI = settings.FlowURI
	}

	def, res, err := flowsupport.GetDefinition(flowAction.flowURI)
	if err != nil {
		return nil, err
	}
	if def == nil {
		return nil, errors.New("unable to resolve flow: " + flowAction.flowURI)
	}

	flowAction.ioMetadata = def.Metadata()

	if res {
		flowAction.resFlow = def
	}

	return flowAction, nil
}

type FlowAction struct {
	flowURI    string
	resFlow    *definition.Definition
	ioMetadata *metadata.IOMetadata
	info       *action.Info
}

func (fa *FlowAction) Info() *action.Info {
	return fa.info
}

//Metadata get the Action's metadata
func (fa *FlowAction) Metadata() *action.Metadata {
	return actionMd
}

func (fa *FlowAction) IOMetadata() *metadata.IOMetadata {
	return fa.ioMetadata
}

// Run implements action.Action.Run
func (fa *FlowAction) Run(context context.Context, inputs map[string]interface{}, handler action.ResultHandler) error {

	op := instance.OpStart
	retID := false
	var initialState *instance.IndependentInstance
	var flowURI string

	runOptions, exists := inputs["_run_options"]

	var execOptions *instance.ExecOptions

	if exists {
		ro, ok := runOptions.(*instance.RunOptions)

		if ok {
			op = ro.Op
			retID = ro.ReturnID
			initialState = ro.InitialState
			flowURI = ro.FlowURI
			execOptions = ro.ExecOptions
		}
	}

	delete(inputs, "_run_options")

	if flowURI == "" {
		flowURI = fa.flowURI
	}

	logger.Debugf("Running FlowAction for URI: '%s'", flowURI)

	//todo: catch panic
	//todo: consider switch to URI to dictate flow operation (ex. flow://blah/resume)

	var inst *instance.IndependentInstance

	switch op {
	case instance.OpStart:

		flowDef := fa.resFlow

		if flowDef == nil {
			var err error
			flowDef, err = flowManager.GetFlow(flowURI)
			if err != nil {
				return err
			}

			if flowDef == nil {
				return errors.New("flow not found for URI: " + flowURI)
			}
		}

		instanceID := idGenerator.NextAsString()
		logger.Debug("Creating Flow Instance: ", instanceID)

		instLogger := logger

		if log.CtxLoggingEnabled() {
			instLogger = log.ChildLoggerWithFields(logger, log.String("flowName", flowDef.Name()), log.String("flowId", instanceID))
		}

		inst = instance.NewIndependentInstance(instanceID, flowURI, flowDef, instLogger)
	case instance.OpResume:
		if initialState != nil {
			inst = initialState
			logger.Debug("Resuming Flow Instance: ", inst.ID())
		} else {
			return errors.New("unable to resume instance, initial state not provided")
		}
	case instance.OpRestart:
		if initialState != nil {
			inst = initialState
			instanceID := idGenerator.NextAsString()
			//flowDef, err := manager.GetFlow(flowURI)
			//if err != nil {
			//	return err
			//}

			//if flowDef.Metadata == nil {
			//	//flowDef.SetMetadata(fa.config.Metadata)
			//}
			err := inst.Restart(instanceID, flowManager)
			if err != nil {
				return err
			}

			logger.Debug("Restarting Flow Instance: ", instanceID)
		} else {
			return errors.New("unable to restart instance, initial state not provided")
		}
	}

	if execOptions != nil {
		logger.Debugf("Applying Exec Options to instance: %s", inst.ID())
		instance.ApplyExecOptions(inst, execOptions)
	}

	//todo how do we check if debug is enabled?
	//logInputs(inputs)

	logger.Debugf("Executing Flow Instance: %s", inst.ID())

	if op == instance.OpStart {

		inst.Start(inputs)
	} else {
		inst.UpdateAttrs(inputs)
	}

	stepCount := 0
	hasWork := true

	inst.SetResultHandler(handler)

	go func() {

		defer handler.Done()

		if retID {

			//idAttr, _ := data.NewAttribute("id", data.TypeString, inst.ID())
			results := map[string]interface{}{
				"id": inst.ID(),
			}

			handler.HandleResult(results, nil)
		}

		for hasWork && inst.Status() < model.FlowStatusCompleted && stepCount < maxStepCount {
			stepCount++
			logger.Debugf("Step: %d", stepCount)
			hasWork = inst.DoStep()

			if record {
				ep.GetStateRecorder().RecordSnapshot(inst)
				ep.GetStateRecorder().RecordStep(inst)
			}
		}

		if inst.Status() == model.FlowStatusCompleted {
			returnData, err := inst.GetReturnData()
			handler.HandleResult(returnData, err)
		} else if inst.Status() == model.FlowStatusFailed {
			handler.HandleResult(nil, inst.GetError())
		}

		logger.Debugf("Done Executing flow instance [%s] - Status: %d", inst.ID(), inst.Status())

		if inst.Status() == model.FlowStatusCompleted {
			logger.Infof("Instance [%s] Done", inst.ID())
		} else if inst.Status() == model.FlowStatusFailed {
			logger.Infof("Instance [%s] Failed", inst.ID())
		}
	}()

	return nil
}

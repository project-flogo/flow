package flow

import (
	"context"
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
	"github.com/project-flogo/core/support/trace"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
	_ "github.com/project-flogo/flow/model/simple"
	flowSupport "github.com/project-flogo/flow/support"
	"github.com/project-flogo/flow/tester"
)

const (
	EnvFlowRecord = "FLOGO_FLOW_RECORD"
)

func init() {
	_ = action.Register(&FlowAction{}, &ActionFactory{})
	_ = resource.RegisterLoader(flowSupport.ResTypeFlow, &flowSupport.FlowLoader{})
}

var ep ExtensionProvider
var idGenerator *support.Generator
var record bool
var maxStepCount = 1000000
var actionMd = action.ToMetadata(&Settings{})
var logger log.Logger

var flowManager *flowSupport.FlowManager

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
		testerEnabled := os.Getenv(tester.EnvEnabled)
		if strings.ToLower(testerEnabled) == "true" {
			ep = tester.NewExtensionProvider()

			sm := support.GetDefaultServiceManager()
			err := sm.RegisterService(ep.GetFlowTester())
			if err != nil {
				return err
			}
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
	flowManager = flowSupport.NewFlowManager(ep.GetFlowProvider())
	flowSupport.InitDefaultDefLookup(flowManager, ctx.ResourceManager())

	return nil
}

func recordFlows() bool {
	recordFlows := os.Getenv(EnvFlowRecord)
	if len(recordFlows) == 0 {
		return false
	}
	b, _ := strconv.ParseBool(recordFlows)
	return b
}

func (f *ActionFactory) New(config *action.Config) (action.Action, error) {

	flowAction := &FlowAction{}

	settings := &Settings{}
	err := metadata.MapToStruct(config.Settings, settings, true)
	if err != nil {
		return nil, fmt.Errorf("action settings error: %s", err.Error())
	}

	flowAction.flowURI = settings.FlowURI

	def, res, err := flowSupport.GetDefinition(flowAction.flowURI)
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
func (fa *FlowAction) Run(ctx context.Context, inputs map[string]interface{}, handler action.ResultHandler) error {
	var err error
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
			instLogger = log.ChildLoggerWithFields(logger, log.FieldString("flowName", flowDef.Name()), log.FieldString("flowId", instanceID))
		}

		inst, err = instance.NewIndependentInstance(instanceID, flowURI, flowDef, instLogger)
		if err != nil {
			return err
		}
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

	tc, err := trace.GetTracer().StartTrace(inst.SpanConfig(),trace.ExtractTracingContext(ctx) )
	if err != nil {
		return err
	}
	inst.SetTracingContext(tc)


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
			if inst.TracingContext() != nil {
				_ = trace.GetTracer().FinishTrace(inst.TracingContext(), nil)
			}
			handler.HandleResult(returnData, err)
		} else if inst.Status() == model.FlowStatusFailed {
			if inst.TracingContext() != nil {
				_ = trace.GetTracer().FinishTrace(inst.TracingContext(), inst.GetError())
			}
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

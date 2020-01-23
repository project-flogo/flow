package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/app/resource"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/support/service"
	"github.com/project-flogo/core/support/trace"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/model/simple"
	"github.com/project-flogo/flow/state"
	flowsupport "github.com/project-flogo/flow/support"
)

func init() {
	_ = action.Register(&FlowAction{}, &ActionFactory{})
	_ = resource.RegisterLoader(flowsupport.ResTypeFlow, &flowsupport.FlowLoader{})
}

const (
	RtSettingStepMode     = "stepRecordingMode"
	RtSettingSnapshotMode = "snapshotRecordingMode"
)

var idGenerator *support.Generator
var maxStepCount = 1000000
var actionMd = action.ToMetadata(&Settings{})
var logger log.Logger
var flowManager *flowsupport.FlowManager
var stateRecorder state.Recorder
var recordSnapshot bool //todo switch to "mode"
var recordSteps bool    //todo switch to "mode"

type ActionFactory struct {
	resManager *resource.Manager
}

func (f *ActionFactory) Initialize(ctx action.InitContext) error {

	f.resManager = ctx.ResourceManager()
	logger = log.ChildLogger(log.RootLogger(), "flow")

	if flowManager != nil {
		return nil
	}

	sm := ctx.ServiceManager()

	srService := sm.FindService(func(s service.Service) bool {
		_, ok := s.(state.Recorder)
		return ok
	})

	stepMode := ""
	snapshotMode := ""

	if len(ctx.RuntimeSettings()) > 0 {
		sStepMode := ctx.RuntimeSettings()[RtSettingStepMode]
		sSnapshotMode := ctx.RuntimeSettings()[RtSettingSnapshotMode]

		stepMode, _ = coerce.ToString(sStepMode)
		snapshotMode, _ = coerce.ToString(sSnapshotMode)
	}

	//todo only support "full" for now, until we come up with other modes
	recordSteps = strings.EqualFold("full", stepMode)
	recordSnapshot = strings.EqualFold("full", snapshotMode)

	if srService != nil {
		stateRecorder = srService.(state.Recorder)

		if recordSteps {
			instance.EnableChangeTracking(true)
		}
	}

	exprFactory := expression.NewFactory(definition.GetDataResolver())
	mapperFactory := mapper.NewFactory(definition.GetDataResolver())

	definition.SetMapperFactory(mapperFactory)
	definition.SetExprFactory(exprFactory)

	if idGenerator == nil {
		idGenerator, _ = support.NewGenerator()
	}

	//todo fix the following
	model.RegisterDefault(simple.New())
	flowManager = flowsupport.NewFlowManager(nil)
	flowsupport.InitDefaultDefLookup(flowManager, ctx.ResourceManager())

	return nil
}

func (f *ActionFactory) New(config *action.Config) (action.Action, error) {

	flowAction := &FlowAction{}

	settings := &Settings{}
	err := metadata.MapToStruct(config.Settings, settings, true)
	if err != nil {
		return nil, fmt.Errorf("action settings error: %s", err.Error())
	}

	flowAction.flowURI = settings.FlowURI

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

	if flowURI == "" {
		return fmt.Errorf("cannot run flow, flowURI not specified")
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
	case instance.OpRestart:
		if initialState != nil {

			inst = initialState
			instanceID := idGenerator.NextAsString()

			logger.Debug("Restarting Flow Instance: ", instanceID)

			instLogger := logger
			if log.CtxLoggingEnabled() {
				instLogger = log.ChildLoggerWithFields(logger, log.FieldString("flowName", inst.Name()), log.FieldString("flowId", instanceID))
			}

			err := inst.Restart(instLogger, instanceID)
			if err != nil {
				return err
			}

		} else {
			return errors.New("unable to restart instance, initial state not provided")
		}
	case instance.OpResume:
		if initialState != nil {
			inst = initialState
			logger.Debug("Resuming Flow Instance: ", inst.ID())

			//instLogger := logger
			//
			//if log.CtxLoggingEnabled() {
			//	instLogger = log.ChildLoggerWithFields(logger, log.FieldString("flowName", inst.Name()), log.FieldString("flowId", instanceID))
			//}

		} else {
			return errors.New("unable to resume instance, initial state not provided")
		}
	}

	if execOptions != nil {
		logger.Debugf("Applying Exec Options to instance: %s", inst.ID())
		instance.ApplyExecOptions(inst, execOptions)
	}

	if trace.Enabled() {
		tc, err := trace.GetTracer().StartTrace(inst.SpanConfig(), trace.ExtractTracingContext(ctx))
		if err != nil {
			return err
		}
		inst.SetTracingContext(tc)
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

	if stateRecorder != nil {
		recordState(inst)
	}

	go func() {

		defer handler.Done()

		if retID {

			results := map[string]interface{}{
				"id": inst.ID(),
			}

			handler.HandleResult(results, nil)
		}

		for hasWork && inst.Status() < model.FlowStatusCompleted && stepCount < maxStepCount {
			stepCount++
			logger.Debugf("Step: %d", stepCount)
			hasWork = inst.DoStep()

			if stateRecorder != nil {
				recordState(inst)
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

func recordState(inst *instance.IndependentInstance) {
	if recordSnapshot {
		err := stateRecorder.RecordSnapshot(inst.Snapshot())
		if err != nil {
			logger.Warnf("unable to record snapshot: %v", err)
		}
	}

	if recordSteps {
		err := stateRecorder.RecordStep(inst.CurrentStep(true))
		if err != nil {
			logger.Warnf("unable to record step: %v", err)
		}
	}
}
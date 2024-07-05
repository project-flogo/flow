package flow

import (
	"context"
	"errors"
	"fmt"
	"github.com/project-flogo/core/data/expression/script/gocc/ast"
	"strings"
	"time"

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
	"github.com/project-flogo/core/trigger"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/model/simple"
	"github.com/project-flogo/flow/state"
	flowsupport "github.com/project-flogo/flow/support"
	"github.com/project-flogo/flow/util"
)

func init() {
	_ = action.Register(&FlowAction{}, &ActionFactory{})
	_ = resource.RegisterLoader(flowsupport.ResTypeFlow, &flowsupport.FlowLoader{})
}

const (
	StateRecordingMode = "stateRecordingMode"
	// Deprecated
	RtSettingStepMode     = "stepRecordingMode"
	RtSettingSnapshotMode = "snapshotRecordingMode"
)

var idGenerator *support.Generator
var maxStepCount = util.GetMaxStepCount()
var actionMd = action.ToMetadata(&Settings{})
var logger log.Logger
var flowManager *flowsupport.FlowManager
var stateRecorder state.Recorder
var stateRecordingMode = state.RecordingModeOff

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

	if len(ctx.RuntimeSettings()) > 0 {
		mode, ok := ctx.RuntimeSettings()[StateRecordingMode]
		if !ok {
			// For backward compatible
			sStepMode := ctx.RuntimeSettings()[RtSettingStepMode]
			sSnapshotMode := ctx.RuntimeSettings()[RtSettingSnapshotMode]

			stepMode, _ := coerce.ToString(sStepMode)
			snapshotMode, _ := coerce.ToString(sSnapshotMode)

			recordSteps := strings.EqualFold("full", stepMode)
			recordSnapshot := strings.EqualFold("full", snapshotMode)
			if recordSteps && recordSnapshot {
				stateRecordingMode = state.RecordingModeFull
			} else if recordSteps {
				stateRecordingMode = state.RecordingModeStep
			} else if recordSnapshot {
				stateRecordingMode = state.RecordingModeSnapshot
			} else {
				stateRecordingMode = state.RecordingModeOff
			}
		} else {
			var err error
			stateRecordingMode, err = state.ToRecordingMode(mode)
			if err != nil {
				return nil
			}
		}

	}

	if srService != nil {
		stateRecorder = srService.(state.Recorder)
		if state.RecordSteps(stateRecordingMode) {
			instance.EnableChangeTracking(true, stateRecordingMode)
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

// Metadata get the Action's metadata
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
	var preserveInstanceId, originalInstanceId string
	var initStepId int
	var rerun, detachExecution bool
	runOptions, exists := inputs["_run_options"]

	var execOptions *instance.ExecOptions

	if exists {
		ro, ok := runOptions.(*instance.RunOptions)
		if ok {
			op = ro.Op
			retID = ro.ReturnID
			preserveInstanceId = ro.PreservedInstanceId
			initialState = ro.InitialState
			flowURI = ro.FlowURI
			execOptions = ro.ExecOptions
			initStepId = ro.InitStepId
			rerun = ro.Rerun
			originalInstanceId = ro.OriginalInstanceId
			detachExecution = ro.DetachExecution
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
			flowDef, err = flowManager.GetFlow(flowURI)
			if err != nil {
				return err
			}
			if flowDef == nil {
				return errors.New("flow not found for URI: " + flowURI)
			}
		}

		var instanceID string
		if len(preserveInstanceId) > 0 {
			instanceID = preserveInstanceId
		} else {
			instanceID = idGenerator.NextAsString()
		}

		logger.Debug("Creating Flow Instance: ", instanceID)
		logger.Debugf("Creating Flow Instance [%s] for event id [%s] ", instanceID, trigger.GetHandlerEventIdFromContext(ctx))

		instLogger := logger

		if log.CtxLoggingEnabled() {
			instLogger = log.ChildLoggerWithFields(logger, log.FieldString("flowName", flowDef.Name()), log.FieldString("flowId", instanceID), log.FieldString("eventId", trigger.GetHandlerEventIdFromContext(ctx)))
		}

		inst, err = instance.NewIndependentInstance(instanceID, flowURI, flowDef, instance.NewStateInstanceRecorder(stateRecorder, stateRecordingMode, rerun), instLogger)
		if err != nil {
			return err
		}
	case instance.OpRestart:
		if initialState != nil {

			inst = initialState
			var instanceID string

			if len(preserveInstanceId) > 0 {
				instanceID = preserveInstanceId
			} else {
				instanceID = idGenerator.NextAsString()
			}

			logger.Debug("Restarting Flow Instance: ", instanceID)

			instLogger := logger
			if log.CtxLoggingEnabled() {
				instLogger = log.ChildLoggerWithFields(logger, log.FieldString("flowName", inst.Name()), log.FieldString("flowId", instanceID))
			}
			inst.SetInstanceRecorder(instance.NewStateInstanceRecorder(stateRecorder, stateRecordingMode, rerun))
			//Engine should set init step id one step before current restart step
			err := inst.Restart(instLogger, instanceID, initStepId-1)
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

	//Update flow starting time
	inst.UpdateStartTime()
	if stateRecorder != nil {
		flowState := inst.GetFlowState(inputs)
		flowState.OriginalInstanceId = originalInstanceId
		stateRecorder.RecordStart(flowState)
	}

	eventID := trigger.GetHandlerEventIdFromContext(ctx)
	if eventID != "" {
		// Add eventId to the instance
		_ = inst.SetValue(instance.EventIdAttr, eventID)
	}
	if trace.Enabled() {
		tc, err := trace.GetTracer().StartTrace(inst.SpanConfig(), trace.ExtractTracingContext(ctx))
		if err != nil {
			return err
		}
		if eventID != "" {
			tc.SetTag("flogo_event_id", eventID)
		}
		inst.SetTracingContext(tc)
	}

	//todo how do we check if debug is enabled?
	//logInputs(inputs)
	logger.Infof("Executing Flow Instance [%s] for event id [%s]", inst.ID(), trigger.GetHandlerEventIdFromContext(ctx))

	if op == instance.OpStart {
		inst.Start(inputs)
	} else {
		inst.UpdateAttrs(inputs)
	}

	//initStepId cannot less than 1. restart must start with 1 to xxxx
	stepCount := 0
	if initStepId > 0 {
		stepCount = initStepId - 1
	}

	hasWork := true

	inst.SetResultHandler(handler)
	if stateRecorder != nil {
		//We don't need record step 0 if restart from activity
		if initStepId <= 0 {
			inst.RecordState(time.Now().UTC())
		} else {
			//Just increase the step number
			inst.CurrentStep(true)
		}
	}

	go func() {
		if detachExecution {
			// In detached mode, no reply expected. So, notifying handler.
			handler.Done()
		} else {
			defer handler.Done()
		}

		if retID {

			results := map[string]interface{}{
				"id": inst.ID(),
			}

			handler.HandleResult(results, nil)
		}

		for hasWork && inst.Status() < model.FlowStatusCompleted && stepCount < maxStepCount {
			stepCount++
			logger.Debugf("Step: %d", stepCount)
			taskStartTime := time.Now().UTC()
			hasWork = inst.DoStep()
			if stateRecorder != nil {
				inst.RecordState(taskStartTime)
			}
		}
		if stepCount == maxStepCount && inst.Status() != model.FlowStatusCompleted {
			err := fmt.Errorf("Flow instance [%s] failed due to max step count [%d] reached. Increase step count by setting [%s] to higher value", inst.ID(), maxStepCount, util.FlogoStepCountEnv)
			if inst.TracingContext() != nil {
				_ = trace.GetTracer().FinishTrace(inst.TracingContext(), err)
			}
			handler.HandleResult(nil, err)
			return
		}

		if inst.Status() == model.FlowStatusCompleted {
			returnData, err := inst.GetReturnData()
			if inst.TracingContext() != nil {
				_ = trace.GetTracer().FinishTrace(inst.TracingContext(), nil)
			}
			for k, v := range returnData {
				inst.SetValue(k, v)
			}

			fa.applyAssertionInterceptor(inst, flowsupport.AssertionActivity)

			handler.HandleResult(returnData, err)
		} else if inst.Status() == model.FlowStatusFailed {
			hasFlowExceptionAssert := fa.applyAssertionInterceptor(inst, flowsupport.AssertionException)

			if inst.TracingContext() != nil {
				_ = trace.GetTracer().FinishTrace(inst.TracingContext(), inst.GetError())
			}
			// If we are in unit testing mode and flow has assertion exception then we dont make the flow fail
			// The testcase will be passed or failed based on the assertions executed.
			if hasFlowExceptionAssert {
				handler.HandleResult(nil, nil)
			}
			handler.HandleResult(nil, inst.GetError())
		}

		logger.Debugf("Executing flow instance [%s] for event id [%s] - Status: %d", inst.ID(), trigger.GetHandlerEventIdFromContext(ctx), inst.Status())

		if inst.Status() == model.FlowStatusCompleted {
			logger.Infof("Flow Instance [%s] for event id [%s] completed in %s", inst.ID(), trigger.GetHandlerEventIdFromContext(ctx), inst.ExecutionTime().String())
		} else if inst.Status() == model.FlowStatusFailed {
			logger.Infof("Flow Instance [%s] for event id [%s] failed in %s", inst.ID(), trigger.GetHandlerEventIdFromContext(ctx), inst.ExecutionTime().String())
		}

		if stateRecorder != nil {
			stateRecorder.RecordDone(inst.GetFlowState(inputs))
		}
	}()

	return nil
}

func (fa *FlowAction) applyAssertionInterceptor(inst *instance.IndependentInstance, assertType int) bool {

	if !inst.HasInterceptor() {
		return false
	}
	hasFlowExceptionAssertion := false
	if inst.GetInterceptor() != nil {
		interceptor := inst.GetInterceptor().GetTaskInterceptor(inst.Instance.Name() + "-_flowOutput")
		if interceptor != nil {
			interceptor.Result = flowsupport.Pass
			ef := expression.NewFactory(definition.GetDataResolver())
			for id, assertion := range interceptor.Assertions {
				if interceptor.Type != assertType {
					interceptor.Assertions[id].Result = flowsupport.AssertionNotExecuted
					continue
				}
				if assertion.Expression == "" {
					interceptor.Assertions[id].Message = "Empty expression"
					interceptor.Assertions[id].Result = flowsupport.NotExecuted
					continue
				}

				expr, _ := ef.NewExpr(fmt.Sprintf("%v", assertion.Expression))
				if expr == nil {
					interceptor.Assertions[id].Result = flowsupport.Fail
					interceptor.Assertions[id].Message = "Failed to validate expression"
					continue
				}
				result, err := expr.Eval(inst.Instance)
				if err != nil {
					interceptor.Assertions[id].Result = flowsupport.Fail
					interceptor.Assertions[id].Message = "Failed to evaluate expression"
				} else {
					exp, ok := expr.(ast.ExprEvalResult)
					var resultData ast.ExprEvalData
					if ok {
						resultData = exp.Detail()
					}
					res, _ := coerce.ToBool(result)
					if res {
						interceptor.Assertions[id].Result = flowsupport.Pass
						interceptor.Assertions[id].Message = "Comparison success"
					} else {
						interceptor.Assertions[id].Result = flowsupport.Fail
						interceptor.Assertions[id].Message = "Comparison failure"
					}
					interceptor.Assertions[id].EvalResult = resultData
					if assertType == flowsupport.AssertionException {
						hasFlowExceptionAssertion = true
					}

				}

			}
		}
	}
	return hasFlowExceptionAssertion

}

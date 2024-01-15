package ondemand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/data/resolve"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/support/service"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/model/simple"
	"github.com/project-flogo/flow/state"
	flowsupport "github.com/project-flogo/flow/support"
	"github.com/project-flogo/flow/util"
)

const (
	ivFlowPackage = "flowPackage"
)

const (
	StateRecordingMode = "stateRecordingMode"
	// Deprecated
	RtSettingStepMode     = "stepRecordingMode"
	RtSettingSnapshotMode = "snapshotRecordingMode"
)

type FlowAction struct {
	FlowURI    string
	IoMetadata *metadata.IOMetadata
}

type FlowPackage struct {
	Inputs  map[string]interface{}    `json:"inputs"`
	Outputs map[string]interface{}    `json:"outputs"`
	Flow    *definition.DefinitionRep `json:"flow"`
}

var idGenerator *support.Generator
var record bool
var flowManager *flowsupport.FlowManager
var logger log.Logger
var stateRecorder state.Recorder
var stateRecordingMode = state.RecordingModeOff

var actionMd = action.ToMetadata(&Settings{})
var maxStepCount = util.GetMaxStepCount()

type Settings struct {
}

func init() {
	_ = action.Register(&FlowAction{}, &ActionFactory{})
}

type ActionFactory struct {
}

func (f *ActionFactory) Initialize(ctx action.InitContext) error {

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

// Metadata get the Action's metadata
func (fa *FlowAction) Metadata() *action.Metadata {
	return actionMd
}

func (fa *FlowAction) IOMetadata() *metadata.IOMetadata {
	return fa.IoMetadata
}

func (f *ActionFactory) New(config *action.Config) (action.Action, error) {

	flowAction := &FlowAction{}

	if config.Settings != nil {

	} else {
		flowAction.IoMetadata = &metadata.IOMetadata{Input: nil, Output: nil}
	}

	//temporary hack to support dynamic process running by tester
	//if config.Data == nil {
	//	return flowAction, nil
	//}

	return flowAction, nil

}

// Run implements action.Action.Run
func (fa *FlowAction) Run(ctx context.Context, inputs map[string]interface{}, handler action.ResultHandler) error {

	logger.Info("Running OnDemand Flow Action")

	fpAttr, exists := inputs[ivFlowPackage]

	flowPackage := &FlowPackage{}

	if exists {

		raw := fpAttr.(json.RawMessage)
		err := json.Unmarshal(raw, flowPackage)
		if err != nil {
			return err
		}
	} else {
		return errors.New("flow package not provided")
	}

	logger.Debugf("InputMappings: %+v", flowPackage.Inputs)
	logger.Debugf("OutputMappings: %+v", flowPackage.Outputs)

	flowDef, err := definition.NewDefinition(flowPackage.Flow)

	if err != nil {
		return err
	}

	flowInputs, err := ApplyMappings(flowPackage.Inputs, inputs)
	if err != nil {
		return err
	}

	instanceID := idGenerator.NextAsString()
	logger.Debug("Creating Flow Instance: ", instanceID)

	inst, err := instance.NewIndependentInstance(instanceID, "", flowDef, nil, logger)

	if err != nil {
		return err
	}

	logger.Debugf("Executing Flow Instance: %s", inst.ID())

	inst.Start(flowInputs)

	var stepCount int64 = 0
	hasWork := true

	inst.SetResultHandler(handler)

	go func() {

		defer handler.Done()

		if !inst.FlowDefinition().ExplicitReply() {

			results := make(map[string]interface{})

			results["id"] = inst.ID

			handler.HandleResult(results, nil)
		}

		for hasWork && inst.Status() < model.FlowStatusCompleted && stepCount < maxStepCount {
			stepCount++
			logger.Debugf("Step: %d", stepCount)
			hasWork = inst.DoStep()

			if record {
				//ep.GetStateRecorder().RecordSnapshot(inst)
				//ep.GetStateRecorder().RecordStep(inst)
			}
		}

		if stepCount == maxStepCount && inst.Status() != model.FlowStatusCompleted {
			handler.HandleResult(nil, fmt.Errorf("Flow instance [%s] failed due to max step count [%d] reached. Increase step count by setting [%s] to higher value", inst.ID(), maxStepCount, util.FlogoStepCount))
		}

		if inst.Status() == model.FlowStatusCompleted {
			returnData, err := inst.GetReturnData()

			handler.HandleResult(returnData, err)

		} else if inst.Status() == model.FlowStatusFailed {
			handler.HandleResult(nil, inst.GetError())
		}

		logger.Debugf("Done Executing flow instance [%s] - Status: %d", inst.ID(), inst.Status())

		if inst.Status() == model.FlowStatusCompleted {
			logger.Infof("Flow instance [%s] Completed Successfully", inst.ID())
		} else if inst.Status() == model.FlowStatusFailed {
			logger.Infof("Flow instance [%s] Failed", inst.ID())
		}
	}()

	return nil
}

func ApplyMappings(mappings map[string]interface{}, inputs map[string]interface{}) (map[string]interface{}, error) {

	mapperFactory := mapper.NewFactory(resolve.GetBasicResolver())

	m, err := mapperFactory.NewMapper(mappings)
	if err != nil {
		return nil, err
	}

	inScope := data.NewSimpleScope(inputs, nil)

	out, err := m.Apply(inScope)

	return out, nil
}

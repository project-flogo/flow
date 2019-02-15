package ondemand

import (
	"context"
	
	"encoding/json"
	"os"
	"errors"

	"strconv"
	
	"github.com/project-flogo/flow"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/core/data/expression"
	_ "github.com/project-flogo/flow/model/simple"
	flowsupport "github.com/project-flogo/flow/support"
	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/data/resolve"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/support"
	_ "github.com/project-flogo/contrib/activity/log"
	
	
)

const (
	FLOW_REF = "github.com/project-flogo/contrib/action/flow/ondemand"

	ENV_FLOW_RECORD = "FLOGO_FLOW_RECORD"

	ivFlowPackage = "flowPackage"
)

type FlowAction struct {
	FlowURI    string
	IoMetadata *metadata.IOMetadata
}


type FlowPackage struct {
	Inputs  		map[string]interface{}             `json:"inputs"`
	Outputs  		map[string]interface{}            `json:"outputs"`
	Flow           *definition.DefinitionRep `json:"flow"`
}

var ep flow.ExtensionProvider
var idGenerator *support.Generator
var record bool
var flowManager *flowsupport.FlowManager
var logger log.Logger

var actionMd= action.ToMetadata(&Settings{})
//todo expose and support this properly
var maxStepCount = 1000000

type Settings struct{
}


func init() {
	action.Register(&FlowAction{}, &ActionFactory{})
}

func SetExtensionProvider(provider flow.ExtensionProvider) {
	ep = provider
}

type ActionFactory struct {
}

func (f *ActionFactory) Initialize(ctx action.InitContext) error {

	logger = log.ChildLogger(log.RootLogger(), "flow")

	if flowManager != nil {
		return nil
	}
	
	if ep == nil {
		ep = flow.NewDefaultExtensionProvider()
		record = recordFlows()
	}
	exprFactory := expression.NewFactory(definition.GetDataResolver())
	mapperFactory := mapper.NewFactory(definition.GetDataResolver())

	definition.SetMapperFactory(mapperFactory)
	definition.SetExprFactory(exprFactory)

	if idGenerator == nil {
		idGenerator, _ = support.NewGenerator()
	}

	model.RegisterDefault(ep.GetDefaultFlowModel())

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

//Metadata get the Action's metadata
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
	if config.Data == nil {
		return flowAction, nil
	}

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

	inst := instance.NewIndependentInstance(instanceID, "", flowDef,logger)

	logger.Debugf("Executing Flow Instance: %s", inst.ID())

	inst.Start(flowInputs)

	stepCount := 0
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


	mapperFactory :=  mapper.NewFactory(resolve.GetBasicResolver())

	mapper, err :=  mapperFactory.NewMapper(mappings)

	if err != nil{
		return nil, err
	}

	inScope := data.NewSimpleScope(inputs, nil)

	out,err := mapper.Apply(inScope)

	return out, nil
}

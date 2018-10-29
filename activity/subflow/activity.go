package subflow

import (
	"errors"

	"github.com/TIBCOSoftware/flogo-contrib/action/flow/instance"
	"github.com/TIBCOSoftware/flogo-lib/core/activity"
	"github.com/TIBCOSoftware/flogo-lib/core/data"
	"github.com/TIBCOSoftware/flogo-lib/logger"
)

// log is the default package logger
var log = logger.GetLogger("activity-flogo-subFlow")

const (
	settingFlowURI = "flowURI"
)

// SubFlowActivity is an Activity that is used to start a sub-flow, can only be used within the
// context of an flow
// settings: {flowURI}
// input : {sub-flow's input}
// output: {sub-flow's output}
type SubFlowActivity struct {
	metadata *activity.Metadata
}

// NewActivity creates a new SubFlowActivity
func NewActivity(metadata *activity.Metadata) activity.Activity {
	return &SubFlowActivity{metadata: metadata}
}

// Metadata returns the activity's metadata
func (a *SubFlowActivity) Metadata() *activity.Metadata {
	return a.metadata
}

func (a *SubFlowActivity) IOMetadata(ctx activity.Context) (*data.IOMetadata, error) {
	//todo this can be moved to an "init" to optimize
	setting, set := ctx.GetSetting(settingFlowURI)
	if !set {
		return nil, errors.New("flowURI not set")
	}

	flowURI := setting.(string)

	return instance.GetFlowIOMetadata(flowURI)
}

// Eval implements api.Activity.Eval - Invokes a REST Operation
func (a *SubFlowActivity) Eval(ctx activity.Context) (done bool, err error) {

	//todo move to init
	setting, set := ctx.GetSetting(settingFlowURI)

	if !set {
		return false, errors.New("flowURI not set")
	}

	flowURI := setting.(string)
	log.Debugf("Starting SubFlow: %s", flowURI)

	ioMd, err := instance.GetFlowIOMetadata(flowURI)
	if err != nil {
		return false, err
	}

	inputs := make(map[string]*data.Attribute)

	if ioMd != nil {
		for name, attr := range ioMd.Input {

			value := ctx.GetInput(name)
			newAttr, err := data.NewAttribute(attr.Name(), attr.Type(), value)
			if err != nil {
				return false, err
			}

			inputs[name] = newAttr
		}
	}

	err = instance.StartSubFlow(ctx, flowURI, inputs)

	if err != nil {
		return false, err
	}

	return false, nil
}

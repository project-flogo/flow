package flow

import (
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/instance"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/model/simple"
	"github.com/project-flogo/flow/support"
	"github.com/project-flogo/flow/tester"
)

// Provides the different extension points to the FlowBehavior Action
type ExtensionProvider interface {
	GetStateRecorder() instance.StateRecorder
	GetFlowTester() *tester.RestEngineTester

	GetDefaultFlowModel() *model.FlowModel
	GetFlowProvider() definition.Provider
}

//ExtensionProvider is the extension provider for the flow action
type DefaultExtensionProvider struct {
	flowProvider definition.Provider
	flowModel    *model.FlowModel
}

func NewDefaultExtensionProvider() *DefaultExtensionProvider {
	return &DefaultExtensionProvider{}
}

func (fp *DefaultExtensionProvider) GetFlowProvider() definition.Provider {

	if fp.flowProvider == nil {
		fp.flowProvider = &support.BasicRemoteFlowProvider{}
	}

	return fp.flowProvider
}

func (fp *DefaultExtensionProvider) GetDefaultFlowModel() *model.FlowModel {

	if fp.flowModel == nil {
		fp.flowModel = simple.New()
	}

	return fp.flowModel
}

func (fp *DefaultExtensionProvider) GetStateRecorder() instance.StateRecorder {
	return nil
}

func (fp *DefaultExtensionProvider) GetScriptExprFactory() expression.Factory {
	return nil
}

//todo make FlowTester an interface
func (fp *DefaultExtensionProvider) GetFlowTester() *tester.RestEngineTester {
	return nil
}

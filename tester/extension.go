package tester

import (
	"os"
	"strings"

	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/model/simple"
	"github.com/project-flogo/flow/state"
	flowsupport "github.com/project-flogo/flow/support"
)

const (
	EnvEnabled       = "TESTER_ENABLED"
	EnvSettingPort   = "TESTER_PORT"
	EnvSettingSrHost = "TESTER_SR_SERVER"
)

//ExtensionProvider is the extension provider for the flow action
type TesterProvider struct {
	flowProvider  definition.Provider
	flowModel     *model.FlowModel
	stateRecorder state.Recorder
	flowTester    *RestEngineTester
}

func NewExtensionProvider() *TesterProvider {
	return &TesterProvider{}
}

func (fp *TesterProvider) GetFlowProvider() definition.Provider {
	if fp.flowProvider == nil {
		fp.flowProvider = &flowsupport.BasicRemoteFlowProvider{}
	}

	return fp.flowProvider
}

func (fp *TesterProvider) GetDefaultFlowModel() *model.FlowModel {
	if fp.flowModel == nil {
		fp.flowModel = simple.New()
	}

	return fp.flowModel
}

func (fp *TesterProvider) GetStateRecorder() state.Recorder {

	if fp.stateRecorder == nil {
		config := &support.ServiceConfig{Enabled: true}

		server := os.Getenv(EnvSettingSrHost)

		if server != "" {
			parts := strings.Split(server, ":")

			host := parts[0]
			port := "9090"

			if len(parts) > 1 {
				port = parts[1]
			}

			settings := map[string]string{
				"host": host,
				"port": port,
			}
			config.Settings = settings
		} else {
			config.Enabled = false
		}

		//todo needs to be loaded externally
		//fp.stateRecorder = instance.NewRemoteStateRecorder(config)
	}

	return fp.stateRecorder
}

func (fp *TesterProvider) GetScriptExprFactory() expression.Factory {
	return nil
}

func (fp *TesterProvider) GetFlowTester() *RestEngineTester {

	config := &support.ServiceConfig{Enabled: true}

	settings := map[string]string{
		"port": os.Getenv(EnvSettingPort),
	}
	config.Settings = settings
	return NewRestEngineTester(config)
}

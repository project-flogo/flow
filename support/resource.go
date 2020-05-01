package support

import (
	"encoding/json"
	"fmt"

	"github.com/project-flogo/core/app/resource"
	"github.com/project-flogo/flow/definition"
)

const (
	ResTypeFlow = "flow"
)

type FlowLoader struct {
}

func (*FlowLoader) LoadResource(config *resource.Config) (*resource.Resource, error) {
	var flowDefBytes []byte

	flowDefBytes = config.Data

	var defRep *definition.DefinitionRep
	err := json.Unmarshal(flowDefBytes, &defRep)
	if err != nil {
		return nil, fmt.Errorf("Loading flow resource with id '%s' error: %s", config.ID, err.Error())
	}

	flow, err := materializeFlow(defRep)
	if err != nil {
		return nil, err
	}

	return resource.New(ResTypeFlow, flow), nil
}

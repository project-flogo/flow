package support

import (
	"fmt"
	"github.com/project-flogo/core/app/resource"
	"github.com/project-flogo/flow/definition"
	"strings"
)

// todo fix this
var flowManager *FlowManager
var resManager *resource.Manager

func InitDefaultDefLookup(fManager *FlowManager, rManager *resource.Manager) {
	flowManager = fManager
	resManager = rManager
}

func GetDefinition(flowURI string) (*definition.Definition, bool, error) {

	var def *definition.Definition

	if strings.HasPrefix(flowURI, resource.UriScheme) {

		res := resManager.GetResource(flowURI)

		if res != nil {
			var ok bool
			def, ok = res.Object().(*definition.Definition)
			if ok {
				return def, true, nil
			}
		}
	} else {
		var err error
		def, err = flowManager.GetFlow(flowURI)
		if err != nil {
			return nil, false, err
		}

		return def, false, nil
	}

	return nil, false, nil
}

func SetResource(resources []*resource.Config) error {
	for _, resConfig := range resources {
		resType, err := resource.GetTypeFromID(resConfig.ID)
		if err != nil {
			return err
		}

		loader := resource.GetLoader(resType)

		if loader == nil {
			return fmt.Errorf("resource loader for '%s' not registered", resType)
		}

		res, err := loader.LoadResource(resConfig)
		if err != nil {
			return err
		}
		resManager.SetResource(resConfig.ID, res)
	}
	return nil
}

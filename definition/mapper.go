package definition

import (
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/mapper"
)

var mapperFactory mapper.Factory

func SetMapperFactory(factory mapper.Factory) {
	mapperFactory = factory
}

func GetMapperFactory() mapper.Factory {

	if mapperFactory == nil {
		mapperFactory = mapper.NewFactory(GetDataResolver())
	}
	return mapperFactory
}

func NewDefaultActivityOutputMapper(task *Task) mapper.Mapper {
	attrNS := "_A." + task.ID() + "."
	return &defaultActivityOutputMapper{attrNS: attrNS, metadata: task.activityCfg.Activity.Metadata()}
}

// BasicMapper is a simple object holding and executing mappings
type defaultActivityOutputMapper struct {
	attrNS   string
	metadata *activity.Metadata
}

func (m *defaultActivityOutputMapper) Apply(scope data.Scope) (map[string]interface{}, error) {

	if m.metadata.IOMetadata != nil {
		output := make(map[string]interface{}, len(m.metadata.Output))
		for name := range m.metadata.Output {

			value, ok := scope.GetValue(name)
			if ok {
				output[m.attrNS+name] = value
			}
		}

		return output, nil
	}

	return nil, nil
}

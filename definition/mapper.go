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

//
//// MapperDef represents a Mapper, which is a collection of mappings
//type MapperDef struct {
//	Mappings []*data.MappingDef
//}
//
//type MapperFactory interface {
//	// NewMapper creates a new Mapper from the specified MapperDef
//	NewMapper(mapperDef *MapperDef) mapper.Mapper
//
//	// NewActivityInputMapper creates a new Activity Input Mapper from the specified MapperDef
//	// for the specified Task, method to facilitate pre-compiled mappers
//	NewActivityInputMapper(task *Task, mapperDef *mapper.MapperDef) mapper.Mapper
//
//	// NewActivityOutputMapper creates a new Activity Output Mapper from the specified MapperDef
//	// for the specified Task, method to facilitate pre-compiled mappers
//	NewActivityOutputMapper(task *Task, mapperDef *mapper.MapperDef) mapper.Mapper
//
//	// GetDefaultTaskOutputMapper get the default Activity Output Mapper for the
//	// specified Task
//	GetDefaultActivityOutputMapper(task *Task) mapper.Mapper
//
//	// NewTaskInputMapper creates a new Input Mapper from the specified MapperDef
//	// for the specified Task, method to facilitate pre-compiled mappers
//	//Deprecated
//	NewTaskInputMapper(task *Task, mapperDef *MapperDef) mapper.Mapper
//
//	// NewTaskOutputMapper creates a new Output Mapper from the specified MapperDef
//	// for the specified Task, method to facilitate pre-compiled mappers
//	//Deprecated
//	NewTaskOutputMapper(task *Task, mapperDef *MapperDef) mapper.Mapper
//
//	// GetDefaultTaskOutputMapper get the default Output Mapper for the
//	// specified Task
//	//Deprecated
//	GetDefaultTaskOutputMapper(task *Task) mapper.Mapper
//}
//
//var mapperFactory MapperFactory
//
//func SetMapperFactory(factory MapperFactory) {
//	mapperFactory = factory
//
//	baseFactory, ok := interface{}(factory).(mapper.Factory)
//	if ok {
//		mapper.SetFactory(baseFactory)
//	}
//}
//
//func GetMapperFactory() MapperFactory {
//
//	//temp hack until we consolidate mapper definition
//	if mapperFactory == nil {
//		mapperFactory = &BasicMapperFactory{baseFactory: mapper.GetFactory()}
//	}
//
//	return mapperFactory
//}
//
//type BasicMapperFactory struct {
//	baseFactory mapper.Factory
//}
//
//func (mf *BasicMapperFactory) NewMapper(mapperDef *MapperDef) mapper.Mapper {
//	return mf.baseFactory.NewMapper(&mapper.MapperDef{Mappings: mapperDef.Mappings}, GetDataResolver())
//}
//
//func (mf *BasicMapperFactory) NewActivityInputMapper(task *Task, mapperDef *mapper.MapperDef) mapper.Mapper {
//	id := task.definition.name + "." + task.id + ".input"
//	return mf.baseFactory.NewUniqueMapper(id, mapperDef, GetDataResolver())
//}
//
//func (mf *BasicMapperFactory) NewActivityOutputMapper(task *Task, mapperDef *mapper.MapperDef) mapper.Mapper {
//	id := task.definition.name + "." + task.id + ".output"
//	return mf.baseFactory.NewUniqueMapper(id, mapperDef, nil)
//}
//
//func (mf *BasicMapperFactory) GetDefaultActivityOutputMapper(task *Task) mapper.Mapper {
//	act := task.activityCfg.Activity
//	attrNS := "_A." + task.ID() + "."
//
//	if act.Metadata().DynamicIO {
//
//		return &DefaultActivityOutputMapper{attrNS: attrNS, task: task}
//		////todo validate dynamic on instantiation
//		//dynamic, _ := act.(activity.DynamicIO)
//		//dynamicIO, _ := dynamic.IOMetadata(&DummyTaskCtx{task: task})
//		////todo handler error
//		//if dynamicIO != nil {
//		//	return &DefaultActivityOutputMapper{attrNS: attrNS, outputMetadata: dynamicIO.Output}
//		//}
//	}
//
//	return &DefaultActivityOutputMapper{attrNS: attrNS, outputMetadata: act.Metadata().Output}
//}

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

	output := make(map[string]interface{}, len(m.metadata.Output))
	for name := range m.metadata.Output {

		value, ok := scope.GetValue(name)
		if ok {
			output[m.attrNS+name] = value
		}
	}

	return output, nil
}

//
////Deprecated
//func (mf *BasicMapperFactory) NewTaskInputMapper(task *Task, mapperDef *MapperDef) mapper.Mapper {
//	id := task.definition.name + "." + task.id + ".input"
//	return mf.baseFactory.NewUniqueMapper(id, &mapper.MapperDef{Mappings: mapperDef.Mappings}, GetDataResolver())
//}
//
////Deprecated
//func (mf *BasicMapperFactory) NewTaskOutputMapper(task *Task, mapperDef *MapperDef) mapper.Mapper {
//	id := task.definition.name + "." + task.id + ".output"
//	return mf.baseFactory.NewUniqueMapper(id, &mapper.MapperDef{Mappings: mapperDef.Mappings}, nil)
//}
//
////Deprecated
//func (mf *BasicMapperFactory) GetDefaultTaskOutputMapper(task *Task) mapper.Mapper {
//	return &DefaultTaskOutputMapper{task: task}
//}
//
//// BasicMapper is a simple object holding and executing mappings
////Deprecated
//type DefaultTaskOutputMapper struct {
//	task *Task
//}
//
////Deprecated
//func (m *DefaultTaskOutputMapper) Apply(inputScope data.Scope, outputScope data.Scope) error {
//
//	oscope := outputScope.(data.MutableScope)
//
//	act := activity.Get(m.task.ActivityConfig().Ref())
//
//	attrNS := "_A." + m.task.ID() + "."
//
//	for _, attr := range act.Metadata().Output {
//
//		oAttr, _ := inputScope.GetAttr(attr.Name())
//
//		if oAttr != nil {
//			oscope.AddAttr(attrNS+attr.Name(), attr.Type(), oAttr.TypedValue())
//		}
//	}
//
//	return nil
//}
//
////Temporary hack for determining dynamic default outputs
//
//type DummyTaskCtx struct {
//	task *Task
//}
//
//func (*DummyTaskCtx) ActivityHost() activity.Host {
//	return nil
//}
//
//func (ctx *DummyTaskCtx) Name() string {
//	return ctx.task.Name()
//}
//
//func (ctx *DummyTaskCtx) GetSetting(setting string) (value interface{}, exists bool) {
//	val, found := ctx.task.ActivityConfig().GetSetting(setting)
//	if found {
//		return val.TypedValue(), true
//	}
//
//	return nil, false
//}
//
//func (*DummyTaskCtx) GetInitValue(key string) (value interface{}, exists bool) {
//	return nil, false
//}
//
//func (*DummyTaskCtx) GetInput(name string) interface{} {
//	return ""
//}
//
//func (*DummyTaskCtx) GetOutput(name string) interface{} {
//	return nil
//}
//
//func (*DummyTaskCtx) SetOutput(name string, value interface{}) {
//}
//
//func (*DummyTaskCtx) GetSharedTempData() map[string]interface{} {
//	return nil
//}
//
//func (ctx *DummyTaskCtx) TaskName() string {
//	return ctx.task.Name()
//}
//
//func (*DummyTaskCtx) FlowDetails() activity.FlowDetails {
//	return nil
//}

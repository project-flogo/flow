package instance

import (
	"strings"

	"github.com/project-flogo/core/data"
)

type IteratorScope struct {
	iteratorData map[string]interface{}
	parent       data.Scope
}

func (s *IteratorScope) GetValue(name string) (value interface{}, exists bool) {
	if strings.HasPrefix(name, "$current.") {
		val, ok := s.iteratorData[name[9:]]
		if ok {
			return val, true
			//attr, _ = data.NewAttribute(attrName[6:], data.ANY, val)
			//return attr, true
		}
		return nil, false
	} else {
		return s.parent.GetValue(name)
	}
}

func (s *IteratorScope) SetValue(name string, value interface{}) error {
	return s.parent.SetValue(name, value)
}

//
func NewIteratorScope(parentScope data.Scope, workingData map[string]interface{}) data.Scope {

	scope := &IteratorScope{
		parent:       parentScope,
		iteratorData: workingData,
	}

	return scope
}

//
//// WorkingDataScope is scope restricted by the set of reference attrs and backed by the specified Task
//type WorkingDataScope struct {
//	parent      data.Scope
//	workingData map[string]*data.Attribute
//}
//
//// NewFixedTaskScope creates a FixedTaskScope
//func NewWorkingDataScope(parentScope data.Scope, workingData map[string]*data.Attribute) data.Scope {
//
//	scope := &WorkingDataScope{
//		parent:      parentScope,
//		workingData: workingData,
//	}
//
//	return scope
//}
//
//// GetAttr implements Scope.GetAttr
//func (s *WorkingDataScope) GetAttr(attrName string) (attr *data.Attribute, exists bool) {
//
//	if strings.HasPrefix(attrName, "$current.") {
//		val, ok := s.workingData[attrName[9:]]
//		if ok {
//			return val, true
//			//attr, _ = data.NewAttribute(attrName[6:], data.ANY, val)
//			//return attr, true
//		}
//		return nil, false
//	} else {
//		return s.parent.GetAttr(attrName)
//	}
//}
//
//// SetAttrValue implements Scope.SetAttrValue
//func (s *WorkingDataScope) SetAttrValue(attrName string, value interface{}) error {
//	return s.parent.SetAttrValue(attrName, value)
//}
//
//// FixedTaskScope is scope restricted by the set of reference attrs and backed by the specified Task
//type FixedTaskScope struct {
//	attrs       map[string]*data.Attribute
//	refAttrs    map[string]*data.Attribute
//	activityCfg *definition.ActivityConfig
//	isInput     bool
//}
//
//// NewFixedTaskScope creates a FixedTaskScope
//func NewFixedTaskScope(refAttrs map[string]*data.Attribute, task *definition.Task, isInput bool) data.Scope {
//
//	scope := &FixedTaskScope{
//		refAttrs: refAttrs,
//		isInput:  isInput,
//	}
//
//	if task != nil {
//		scope.activityCfg = task.ActivityConfig()
//	}
//
//	return scope
//}
//
//// GetAttr implements Scope.GetAttr
//func (s *FixedTaskScope) GetAttr(attrName string) (attr *data.Attribute, exists bool) {
//
//	if len(s.attrs) > 0 {
//
//		attr, found := s.attrs[attrName]
//
//		if found {
//			return attr, true
//		}
//	}
//
//	if s.activityCfg != nil {
//
//		var attr *data.Attribute
//		var found bool
//
//		if s.isInput {
//			attr, found = s.activityCfg.GetInputAttr(attrName)
//		} else {
//			attr, found = s.activityCfg.GetOutputAttr(attrName)
//		}
//
//		if !found {
//			attr, found = s.refAttrs[attrName]
//		}
//
//		return attr, found
//	}
//
//	return nil, false
//}
//
//// SetAttrValue implements Scope.SetAttrValue
//func (s *FixedTaskScope) SetAttrValue(attrName string, value interface{}) error {
//
//	if len(s.attrs) == 0 {
//		s.attrs = make(map[string]*data.Attribute)
//	}
//
//	logger.Debugf("SetAttr: %s = %v", attrName, value)
//
//	attr, found := s.attrs[attrName]
//
//	var err error
//	if found {
//		err = attr.SetValue(value)
//	} else {
//		// look up reference for type
//		attr, found = s.refAttrs[attrName]
//		if found {
//			s.attrs[attrName], err = data.NewAttribute(attrName, attr.Type(), value)
//		} else {
//			logger.Debugf("SetAttr: Attr '%s' not found in metadata\n", attrName)
//			logger.Debugf("SetAttr: metadata %v\n", s.refAttrs)
//		}
//		//todo: else error
//	}
//
//	return err
//}

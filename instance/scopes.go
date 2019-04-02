package instance

import (
	"strings"

	"github.com/project-flogo/core/data"
)

type WorkingDataScope struct {
	workingData map[string]interface{}
	parent      data.Scope
}

func (s *WorkingDataScope) GetValue(name string) (value interface{}, exists bool) {
	if strings.HasPrefix(name, "_W.") {
		val, ok := s.workingData[name[3:]]
		if ok {
			return val, true
		}
		return nil, false
	} else {
		return s.parent.GetValue(name)
	}
}

func (s *WorkingDataScope) SetValue(name string, value interface{}) error {
	return s.parent.SetValue(name, value)
}

func (s *WorkingDataScope) GetWorkingValue(name string) (value interface{}, exists bool) {
	val, ok := s.workingData[name]
	if ok {
		return val, true
	}
	return nil, false
}

func (s *WorkingDataScope) SetWorkingValue(name string, value interface{}) {
	s.workingData[name] = value
}

// NewWorkingDataScope
func NewWorkingDataScope(parentScope data.Scope) *WorkingDataScope {

	scope := &WorkingDataScope{
		parent:      parentScope,
		workingData: make(map[string]interface{}),
	}

	return scope
}

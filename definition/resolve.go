package definition

import (
	"fmt"
	"strings"

	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/path"
	"github.com/project-flogo/core/data/property"
	"github.com/project-flogo/core/data/resolve"
)

var defResolver = resolve.NewCompositeResolver(map[string]resolve.Resolver{
	".":         &resolve.ScopeResolver{},
	"env":       &resolve.EnvResolver{},
	"property":  &property.Resolver{},
	"loop":      &resolve.LoopResolver{},
	"iteration": &IteratorResolver{}, //todo should we create a separate resolver to use in iterations?
	"activity":  &ActivityResolver{},
	"flowctx":   &FlowContextResolver{},
	"error":     &ErrorResolver{},
	"flow":      &FlowResolver{},
	"variable":  &VariableResolver{},
})

func GetDataResolver() resolve.CompositeResolver {
	return defResolver
}

var resolverInfo = resolve.NewResolverInfo(false, false)

type FlowResolver struct {
}

func (r *FlowResolver) GetResolverInfo() *resolve.ResolverInfo {
	return resolverInfo
}

func (r *FlowResolver) Resolve(scope data.Scope, itemName, valueName string) (interface{}, error) {

	value, exists := scope.GetValue(valueName)
	if !exists {
		return nil, fmt.Errorf("failed to resolve flow attr: '%s', not found in flow", valueName)
	}
	return value, nil
}

var dynamicItemResolver = resolve.NewResolverInfo(false, true)

type ActivityResolver struct {
}

func (r *ActivityResolver) GetResolverInfo() *resolve.ResolverInfo {
	return dynamicItemResolver
}

func (r *ActivityResolver) Resolve(scope data.Scope, itemName, valueName string) (interface{}, error) {
	var value interface{}
	var exists bool
	if len(valueName) > 0 {
		value, exists = scope.GetValue("_A." + itemName + "." + valueName)
		if !exists {
			return nil, fmt.Errorf("failed to resolve activity attr: '%s', not found in activity '%s'", valueName, itemName)
		}
	} else {
		//For accumulate or direct root object mapping
		value, exists = scope.GetValue("_A." + itemName)
		if !exists {
			return nil, fmt.Errorf("failed to resolve activity value: '%s'", itemName)
		}
		// Special handling for the case where $activity[ActivityName] used directly in the mappings e.g. coerce.toString($activity[Mapper])
		// For memory optimazation, we only store the activity output mappings in the root object map[string]string e.g. map["_A.<ActivityName>.<outputName>"] = ""
		// and resolved attribute values here
		objValue, ok := value.(map[string]string)
		if ok {
			rootObjValue := make(map[string]interface{}, len(objValue))
			for name := range objValue {
				value, exists = scope.GetValue(name)
				if exists {
					attrName := strings.TrimPrefix(name, "_A."+itemName+".")
					if attrName != "" {
						rootObjValue[attrName] = value
					}
				}
			}
			return rootObjValue, nil
		}
	}

	return value, nil
}

type VariableResolver struct {
}

func (r *VariableResolver) GetResolverInfo() *resolve.ResolverInfo {
	return dynamicItemResolver
}
func (r *VariableResolver) Resolve(scope data.Scope, itemName, valueName string) (interface{}, error) {
	var value interface{}
	var exists bool
	var err error
	var valueMain interface{}
	if len(valueName) > 0 {
		value, exists = scope.GetValue("_V." + itemName + "." + valueName)
		if !exists {
			valueMain, exists = scope.GetValue("_V." + itemName)
			value, err = path.GetValue(valueMain, "."+valueName)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve variable attr: '%s', not found in variable", valueName)
			}
		}
	} else {
		//For accumulate or direct root object mapping
		value, exists = scope.GetValue("_V." + itemName)
		if !exists {
			return nil, fmt.Errorf("failed to resolve activity value: '%s'", itemName)
		}
		// Special handling for the case where $activity[ActivityName] used directly in the mappings e.g. coerce.toString($activity[Mapper])
		// For memory optimazation, we only store the activity output mappings in the root object map[string]string e.g. map["_A.<ActivityName>.<outputName>"] = ""
		// and resolved attribute values here
		objValue, ok := value.(map[string]string)
		if ok {
			rootObjValue := make(map[string]interface{}, len(objValue))
			for name := range objValue {
				value, exists = scope.GetValue(name)
				if exists {
					attrName := strings.TrimPrefix(name, "_V."+itemName+".")
					if attrName != "" {
						rootObjValue[attrName] = value
					}
				}
			}
			return rootObjValue, nil
		}
	}

	return value, nil
}

type FlowContextResolver struct {
}

func (r *FlowContextResolver) GetResolverInfo() *resolve.ResolverInfo {
	return dynamicItemResolver
}

func (r *FlowContextResolver) Resolve(scope data.Scope, itemName, valueName string) (interface{}, error) {

	value, exists := scope.GetValue("_fctx." + itemName)

	if !exists {
		return nil, fmt.Errorf("unknown flow context variable: '%s'. supported flow context variables are 'FlowName', 'FlowId', 'ParentFlowName', 'ParentFlowId', 'TraceId' and 'SpanId'", itemName)
	}
	return value, nil
}

var errorResolverInfo = resolve.NewImplicitResolverInfo(false, true)

//var errorResolverInfo = resolve.NewResolverInfoWithOptions(resolve.OptImplicit)

type ErrorResolver struct {
}

func (r *ErrorResolver) GetResolverInfo() *resolve.ResolverInfo {
	return errorResolverInfo
}

func (r *ErrorResolver) Resolve(scope data.Scope, itemName, valueName string) (interface{}, error) {
	//2 cases,  1. $error.code 2. $error[activityName].code
	var value interface{}
	if itemName == "" {
		v, exist := scope.GetValue("_E")
		if !exist {
			return nil, fmt.Errorf("failed to resolve error, not found in flow")
		}
		value = v
	} else {
		v, exists := scope.GetValue("_E." + itemName)
		if !exists {
			return nil, fmt.Errorf("failed to resolve activity [%s] error, not found in flow", itemName)

		}
		value = v
	}

	if valueName == "" {
		return value, nil
	}

	return path.GetValue(value, "."+valueName)
}

type IteratorResolver struct {
}

func (*IteratorResolver) GetResolverInfo() *resolve.ResolverInfo {
	return dynamicItemResolver
}

// Resolve resolved iterator value using  the following syntax:  $iteration[key], or $iteration[value]
func (*IteratorResolver) Resolve(scope data.Scope, item string, field string) (interface{}, error) {
	value, exists := scope.GetValue("_W.iteration")
	if !exists {
		return nil, fmt.Errorf("failed to resolve iteration value, not in an iterator")
	}
	if len(field) > 0 {
		return path.GetValue(value, "."+item+"."+field)
	} else {
		return path.GetValue(value, "."+item)
	}
}

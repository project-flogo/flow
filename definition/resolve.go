package definition

import (
	"fmt"
	"github.com/project-flogo/core/data/path"

	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/resolve"
)

var defResolver = resolve.NewCompositeResolver(map[string]resolve.Resolver{
	".":          &resolve.ScopeResolver{},
	"env":        &resolve.EnvResolver{},
	"property":   &resolve.PropertyResolver{},
	"loop":       &resolve.LoopResolver{},
	"iteration":  &IteratorResolver{}, //todo should we create a separate resolver to use in iterations?
	"accumulate": &AccumulateResolver{},
	"activity":   &ActivityResolver{},
	"error":      &ErrorResolver{},
	"flow":       &FlowResolver{}})

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

	value, exists := scope.GetValue("_A." + itemName + "." + valueName)
	if !exists {
		return nil, fmt.Errorf("failed to resolve activity attr: '%s', not found in activity '%s'", valueName, itemName)
	}

	return value, nil
}

var errorResolverInfo = resolve.NewImplicitResolverInfo(false, true)

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

	return path.GetValue(value, "."+valueName)
}

type IteratorResolver struct {
}

func (*IteratorResolver) GetResolverInfo() *resolve.ResolverInfo {
	return dynamicItemResolver
}

//Resolve resolved iterator value using  the following syntax:  $iteration[key], or $iteration[value]
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

type AccumulateResolver struct {
}

func (*AccumulateResolver) GetResolverInfo() *resolve.ResolverInfo {
	return dynamicItemResolver
}

//Resolve resolved iterator value using  the following syntax:  $accumulate[activityName].outputs
func (*AccumulateResolver) Resolve(scope data.Scope, item string, field string) (interface{}, error) {
	value, exists := scope.GetValue("_accumulate." + item + "." + field)
	if !exists {
		return nil, fmt.Errorf("failed to resolve accumulate value, value not set")
	}
	return value, nil
}

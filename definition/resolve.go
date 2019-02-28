package definition

import (
	"fmt"

	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/resolve"
)

var defResolver = resolve.NewCompositeResolver(map[string]resolve.Resolver{
	".":        &resolve.ScopeResolver{},
	"env":      &resolve.EnvResolver{},
	"property": &resolve.PropertyResolver{},
	"loop":     &resolve.LoopResolver{},
	"activity": &ActivityResolver{},
	"error":    &ErrorResolver{},
	"flow":     &FlowResolver{}})

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

var actResolverInfo = resolve.NewResolverInfo(false, true)

type ActivityResolver struct {
}

func (r *ActivityResolver) GetResolverInfo() *resolve.ResolverInfo {
	return actResolverInfo
}

func (r *ActivityResolver) Resolve(scope data.Scope, itemName, valueName string) (interface{}, error) {

	value, exists := scope.GetValue("_A." + itemName + "." + valueName)
	if !exists {
		return nil, fmt.Errorf("failed to resolve activity attr: '%s', not found in activity '%s'", valueName, itemName)
	}

	return value, nil
}

type ErrorResolver struct {
}

func (r *ErrorResolver) GetResolverInfo() *resolve.ResolverInfo {
	return resolverInfo
}

func (r *ErrorResolver) Resolve(scope data.Scope, itemName, valueName string) (interface{}, error) {

	value, exists := scope.GetValue("_E." + valueName)
	if !exists {
		return nil, fmt.Errorf("failed to resolve error attr: '%s', not found in flow", valueName)
	}

	return value, nil
}

//
//func (r *FlowResolver) Resolve(toResolve string, scope data.Scope) (value interface{}, err error) {
//
//	var details *data.ResolutionDetails
//
//	if strings.HasPrefix(toResolve, "${") {
//		details, err = data.GetResolutionDetailsOld(toResolve)
//	} else if strings.HasPrefix(toResolve, "$") {
//		details, err = data.GetResolutionDetails(toResolve[1:])
//	} else {
//		return data.SimpleScopeResolve(toResolve, scope)
//	}
//
//	if err != nil {
//		return nil, err
//	}
//
//	if details == nil {
//		return nil, fmt.Errorf("unable to determine resolver for %s", toResolve)
//	}
//
//	var exists bool
//
//	switch details.ResolverName {
//	case "property":
//		// Property resolution
//		provider := data.GetPropertyProvider()
//		value, exists = provider.GetProperty(details.Property + details.Path) //should we add the path and reset it to ""
//		if !exists {
//			err := fmt.Errorf("failed to resolve Property: '%s', ensure that property is configured in the application", details.Property)
//			logger.Error(err.Error())
//			return nil, err
//		}
//	case "env":
//		// Environment resolution
//		value, exists = os.LookupEnv(details.Property + details.Path)
//		if !exists {
//			err := fmt.Errorf("failed to resolve Environment Variable: '%s', ensure that variable is configured", details.Property)
//			logger.Error(err.Error())
//			return "", err
//		}
//	case "activity":
//		attr, exists := scope.GetAttr("_A." + details.Item + "." + details.Property)
//		if !exists {
//			return nil, fmt.Errorf("failed to resolve activity attr: '%s', not found in flow", details.Property)
//		}
//		value = attr.TypedValue()
//	case "error":
//		attr, exists := scope.GetAttr("_E." + details.Property)
//		if !exists {
//			return nil, fmt.Errorf("failed to resolve error attr: '%s', not found in flow", details.Property)
//		}
//		value = attr.TypedValue()
//	case "trigger":
//		attr, exists := scope.GetAttr("_T." + details.Property)
//		if !exists {
//			return nil, fmt.Errorf("failed to resolve trigger attr: '%s', not found in flow", details.Property)
//		}
//		value = attr.TypedValue()
//	case "flow":
//		attr, exists := scope.GetAttr(details.Property)
//		if !exists {
//			return nil, fmt.Errorf("failed to resolve flow attr: '%s', not found in flow", details.Property)
//		}
//		value = attr.TypedValue()
//	case "current":
//		attr, exists := scope.GetAttr("$current." + details.Property)
//		if !exists {
//			return nil, fmt.Errorf("failed to resolve current working data: '%s', not found in scope", details.Property)
//		}
//		value = attr.TypedValue()
//	default:
//		return nil, fmt.Errorf("unsupported resolver: %s", details.ResolverName)
//	}
//
//	if details.Path != "" {
//		value, err = data.PathGetValue(value, details.Path)
//		if err != nil {
//			logger.Error(err.Error())
//			return nil, err
//		}
//	}
//
//	return value, nil
//}

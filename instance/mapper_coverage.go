package instance

import (
	"fmt"
	"sort"
	"strings"

	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/flow/definition"
)

// MapperCoverageKey is the reserved key under which mapper level coverage is attached
// to ActivityCoverage.Inputs.
const MapperCoverageKey = "_mapperCoverage"

const (
	mapperConditional = "@conditional"
	mapperOtherwise   = "@otherwise"
	mapperForeach     = "@foreach"
)

// MapperCoverage describes how a control statement (@conditional, @foreach) of an
// input mapping was evaluated.
type MapperCoverage struct {
	// Type is "conditional" or "foreach"
	Type string `json:"type"`
	// Path is the location of the statement within the input mappings
	Path string `json:"path"`
	// Branches lists every branch of a conditional (if / else if / otherwise)
	Branches []BranchCoverage `json:"branches,omitempty"`
	// Source and Iterations are set for foreach statements
	Source     string `json:"source,omitempty"`
	Iterations *int   `json:"iterations,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BranchCoverage describes a single branch of a conditional mapping
type BranchCoverage struct {
	Index      int    `json:"index"`
	Expression string `json:"expression,omitempty"`
	Otherwise  bool   `json:"otherwise,omitempty"`
	// Taken is true when the branch was the one selected by the mapper
	Taken bool   `json:"taken"`
	Error string `json:"error,omitempty"`
}

var mapperCoverageExprFactory expression.Factory

func getCoverageExprFactory() expression.Factory {
	if mapperCoverageExprFactory == nil {
		mapperCoverageExprFactory = expression.NewFactory(definition.GetDataResolver())
	}
	return mapperCoverageExprFactory
}

// collectMapperCoverage evaluates the control statements found in the mappings against scope
func collectMapperCoverage(mappings map[string]interface{}, scope data.Scope) []MapperCoverage {
	var out []MapperCoverage
	walkMapping("", mappings, scope, getCoverageExprFactory(), &out)
	return out
}

func walkMapping(path string, value interface{}, scope data.Scope, ef expression.Factory, out *[]MapperCoverage) {
	switch t := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := t[k]
			childPath := path + "/" + k
			switch {
			case strings.HasPrefix(k, mapperConditional):
				walkConditional(childPath, k, v, scope, ef, out)
			case strings.HasPrefix(k, mapperForeach):
				*out = append(*out, foreachCoverage(childPath, k, scope, ef))
			default:
				walkMapping(childPath, v, scope, ef, out)
			}
		}
	case []interface{}:
		for i, v := range t {
			walkMapping(fmt.Sprintf("%s[%d]", path, i), v, scope, ef, out)
		}
	}
}

func walkConditional(path, key string, value interface{}, scope data.Scope, ef expression.Factory, out *[]MapperCoverage) {
	cov := MapperCoverage{Type: "conditional", Path: path}

	prefix := ""
	if s, e := strings.Index(key, "("), strings.LastIndex(key, ")"); s > 0 && e > s {
		prefix = key[s+1 : e]
	}

	elems, err := coerce.ToArray(value)
	if err != nil {
		cov.Error = err.Error()
		*out = append(*out, cov)
		return
	}

	selected := -1
	var selectedValue interface{}
	otherwiseIdx := -1
	var otherwiseValue interface{}

	for i, elem := range elems {
		obj, err := coerce.ToObject(elem)
		if err != nil || len(obj) != 1 {
			cov.Branches = append(cov.Branches, BranchCoverage{Index: i, Error: "invalid conditional element"})
			continue
		}
		for expr, branchValue := range obj {
			if expr == mapperOtherwise {
				otherwiseIdx, otherwiseValue = i, branchValue
				cov.Branches = append(cov.Branches, BranchCoverage{Index: i, Otherwise: true})
				continue
			}

			branch := BranchCoverage{Index: i, Expression: expr}
			if selected < 0 {
				full := expr
				if prefix != "" {
					full = prefix + " " + expr
				}
				e, err := ef.NewExpr(full)
				if err == nil {
					var res interface{}
					res, err = e.Eval(scope)
					if err == nil {
						if ok, _ := coerce.ToBool(res); ok {
							branch.Taken = true
							selected, selectedValue = i, branchValue
						}
					}
				}
				if err != nil {
					branch.Error = err.Error()
				}
			}
			cov.Branches = append(cov.Branches, branch)
		}
	}

	if selected < 0 && otherwiseIdx >= 0 {
		selected, selectedValue = otherwiseIdx, otherwiseValue
		for i := range cov.Branches {
			if cov.Branches[i].Index == otherwiseIdx {
				cov.Branches[i].Taken = true
			}
		}
	}

	*out = append(*out, cov)

	// nested control statements are only meaningful in the branch that was taken
	if selected >= 0 {
		walkMapping(fmt.Sprintf("%s[%d]", path, selected), selectedValue, scope, ef, out)
	}
}

func foreachCoverage(path, key string, scope data.Scope, ef expression.Factory) MapperCoverage {
	cov := MapperCoverage{Type: "foreach", Path: path}

	cov.Source = foreachSource(key)
	if cov.Source == "" {
		return cov
	}

	e, err := ef.NewExpr(cov.Source)
	if err != nil {
		cov.Error = err.Error()
		return cov
	}
	res, err := e.Eval(scope)
	if err != nil {
		cov.Error = err.Error()
		return cov
	}
	arr, err := coerce.ToArray(res)
	if err != nil {
		cov.Error = err.Error()
		return cov
	}
	n := len(arr)
	cov.Iterations = &n
	return cov
}

// foreachSource extracts the source expression from "@foreach(source, scopeName, filter)"
func foreachSource(key string) string {
	key = strings.TrimSpace(key)
	s, e := strings.Index(key, "("), strings.LastIndex(key, ")")
	if s < 0 || e <= s {
		return ""
	}
	args := key[s+1 : e]
	depth := 0
	for i, c := range args {
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				return strings.TrimSpace(args[:i])
			}
		}
	}
	return strings.TrimSpace(args)
}

// withMapperCoverage returns inputs with the mapper coverage attached under MapperCoverageKey.
// The original inputs map is never modified.
func withMapperCoverage(inputs map[string]interface{}, cov []MapperCoverage) map[string]interface{} {
	if len(cov) == 0 {
		return inputs
	}
	merged := make(map[string]interface{}, len(inputs)+1)
	for k, v := range inputs {
		merged[k] = v
	}
	merged[MapperCoverageKey] = cov
	return merged
}

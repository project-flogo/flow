package instance

import (
	"testing"

	"github.com/project-flogo/core/data"
	_ "github.com/project-flogo/core/data/expression/script"
)

func TestCollectMapperCoverage(t *testing.T) {
	scope := data.NewSimpleScope(map[string]interface{}{"a": 5, "list": []interface{}{1, 2, 3}}, nil)
	mappings := map[string]interface{}{
		"x": map[string]interface{}{
			"@conditional": []interface{}{
				map[string]interface{}{"$.a > 10": "big"},
				map[string]interface{}{"$.a > 1": "mid"},
				map[string]interface{}{"@otherwise": "small"},
			},
		},
		"y": map[string]interface{}{
			"@foreach($.list, i)": map[string]interface{}{"=": "$loop"},
		},
	}

	cov := collectMapperCoverage(mappings, scope)
	if len(cov) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(cov), cov)
	}
	var cond, fe MapperCoverage
	for _, c := range cov {
		if c.Type == "conditional" {
			cond = c
		} else {
			fe = c
		}
	}
	if len(cond.Branches) != 3 || cond.Branches[0].Taken || !cond.Branches[1].Taken || cond.Branches[2].Taken {
		t.Fatalf("unexpected branches: %+v (err %s)", cond.Branches, cond.Error)
	}
	if fe.Iterations == nil || *fe.Iterations != 3 {
		t.Fatalf("unexpected foreach coverage: %+v", fe)
	}
}

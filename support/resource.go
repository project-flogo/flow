package support

import (
	"encoding/json"
	"fmt"

	"github.com/project-flogo/core/app/resource"
	"github.com/project-flogo/flow/definition"
)

const (
	ResTypeFlow = "flow"
)

type FlowLoader struct {
}

func (*FlowLoader) LoadResource(config *resource.Config) (*resource.Resource, error) {
	var flowDefBytes []byte

	// flowDefBytes = config.Data  // this is previouse logic before normalization, kept for reference

	flowDefBytes = normalizeLegacySchemaValues(config.Data) // normalize wrong schema values to ensure backward compatibility with older flow definitions

	var defRep *definition.DefinitionRep
	err := json.Unmarshal(flowDefBytes, &defRep)
	if err != nil {
		return nil, fmt.Errorf("error loading flow resource with id '%s': %s", config.ID, err.Error())
	}

	flow, err := materializeFlow(defRep)
	if err != nil {
		return nil, err
	}

	return resource.New(ResTypeFlow, flow), nil
}

func normalizeLegacySchemaValues(flowDefBytes []byte) []byte {
	var flowRep interface{}
	if err := json.Unmarshal(flowDefBytes, &flowRep); err != nil {
		return flowDefBytes
	}

	normalizeSchemaValueInTree(flowRep)

	normalized, err := json.Marshal(flowRep)
	if err != nil {
		return flowDefBytes
	}

	return normalized
}

func normalizeSchemaValueInTree(node interface{}) {
	switch n := node.(type) {
	case map[string]interface{}:
		if schemaRep, ok := n["schema"].(map[string]interface{}); ok {
			normalizeSingleSchemaValue(schemaRep, n["type"])
		}

		for _, child := range n {
			normalizeSchemaValueInTree(child)
		}
	case []interface{}:
		for _, child := range n {
			normalizeSchemaValueInTree(child)
		}
	}
}

func normalizeSingleSchemaValue(schemaRep map[string]interface{}, ownerType interface{}) {
	typeName, ok := ownerType.(string)
	if !ok || typeName != "object" {
		return
	}

	schemaType, ok := schemaRep["type"].(string)
	if !ok || schemaType != "json" {
		return
	}

	schemaValue, ok := schemaRep["value"].(string)
	if !ok || schemaValue == "" {
		return
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(schemaValue), &obj); err != nil {
		return
	}

	if len(obj) == 0 || hasJSONSchemaKeywords(obj) || !looksLikePropertyMap(obj) {
		return
	}

	obj = normalizeLegacyRefs(obj)

	wrapped := map[string]interface{}{
		"type":       "object",
		"properties": obj,
	}

	normalized, err := json.Marshal(wrapped)
	if err != nil {
		return
	}

	schemaRep["value"] = string(normalized)
}

func looksLikePropertyMap(obj map[string]interface{}) bool {
	for _, v := range obj {
		child, ok := v.(map[string]interface{})
		if !ok {
			return false
		}

		if ref, ok := child["$ref"].(string); ok && ref != "" {
			continue
		}

		t, ok := child["type"].(string)
		if !ok || t == "" {
			return false
		}
	}

	return true
}

func hasJSONSchemaKeywords(obj map[string]interface{}) bool {
	// Definite schema-only keywords.
	for _, key := range []string{"$schema", "$id", "$ref", "definitions", "items", "oneOf", "anyOf", "allOf", "not", "enum", "additionalProperties"} {
		if _, ok := obj[key]; ok {
			return true
		}
	}

	// 'type' keyword is valid only when string or array of strings.
	if v, ok := obj["type"]; ok {
		if _, isString := v.(string); isString || isStringArray(v) {
			return true
		}
	}

	// 'required' keyword is valid only when array of strings.
	if v, ok := obj["required"]; ok {
		if isStringArray(v) {
			return true
		}
	}

	// If there is a top-level 'properties' and a valid top-level 'type', treat as schema.
	if _, hasProperties := obj["properties"]; hasProperties {
		if t, ok := obj["type"]; ok {
			if _, isString := t.(string); isString || isStringArray(t) {
				return true
			}
		}
	}

	return false
}

func isStringArray(v interface{}) bool {
	a, ok := v.([]interface{})
	if !ok || len(a) == 0 {
		return false
	}

	for _, item := range a {
		if _, ok := item.(string); !ok {
			return false
		}
	}

	return true
}

func normalizeLegacyRefs(properties map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(properties))

	for k, v := range properties {
		child, ok := v.(map[string]interface{})
		if !ok {
			normalized[k] = v
			continue
		}

		if ref, ok := child["$ref"].(string); ok && ref != "" {
			normalized[k] = map[string]interface{}{"type": "object"}
			continue
		}

		// Legacy schemas may contain array items with $ref but no definitions.
		if t, ok := child["type"].(string); ok && t == "array" {
			if items, ok := child["items"].(map[string]interface{}); ok {
				if ref, ok := items["$ref"].(string); ok && ref != "" {
					normalized[k] = map[string]interface{}{
						"type":  "array",
						"items": map[string]interface{}{"type": "object"},
					}
					continue
				}
			}
		}

		normalized[k] = child
	}

	return normalized
}

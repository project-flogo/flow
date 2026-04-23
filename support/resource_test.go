package support

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── isStringArray ────────────────────────────────────────────────────────────

func TestIsStringArray(t *testing.T) {
	assert.False(t, isStringArray(nil))
	assert.False(t, isStringArray("string"))
	assert.False(t, isStringArray(42))
	assert.False(t, isStringArray([]interface{}{}), "empty slice should be false")
	assert.True(t, isStringArray([]interface{}{"a", "b"}))
	assert.False(t, isStringArray([]interface{}{"a", 1}), "mixed types should be false")
	assert.False(t, isStringArray([]interface{}{nil}), "nil element should be false")
}

// ── hasJSONSchemaKeywords ────────────────────────────────────────────────────

func TestHasJSONSchemaKeywords_DefiniteKeywords(t *testing.T) {
	definite := []string{
		"$schema", "$id", "$ref", "definitions", "items",
		"oneOf", "anyOf", "allOf", "not", "enum", "additionalProperties",
	}
	for _, key := range definite {
		assert.True(t, hasJSONSchemaKeywords(map[string]interface{}{key: "v"}), "key %q should match", key)
	}
}

func TestHasJSONSchemaKeywords_TypeKeyword(t *testing.T) {
	// string value → schema keyword
	assert.True(t, hasJSONSchemaKeywords(map[string]interface{}{"type": "object"}))
	// string-array value → schema keyword
	assert.True(t, hasJSONSchemaKeywords(map[string]interface{}{"type": []interface{}{"string", "null"}}))
	// non-string, non-array value → not a schema keyword
	assert.False(t, hasJSONSchemaKeywords(map[string]interface{}{"type": 42}))
	// bool value → not treated as schema keyword
	assert.False(t, hasJSONSchemaKeywords(map[string]interface{}{"type": true}))
}

func TestHasJSONSchemaKeywords_RequiredKeyword(t *testing.T) {
	// string-array value → schema keyword
	assert.True(t, hasJSONSchemaKeywords(map[string]interface{}{"required": []interface{}{"field1", "field2"}}))
	// non-string-array → not a schema keyword
	assert.False(t, hasJSONSchemaKeywords(map[string]interface{}{"required": []interface{}{1, 2}}))
	// empty slice → not a schema keyword (isStringArray returns false for empty)
	assert.False(t, hasJSONSchemaKeywords(map[string]interface{}{"required": []interface{}{}}))
}

func TestHasJSONSchemaKeywords_PropertiesWithType(t *testing.T) {
	// properties + string type → schema keyword
	assert.True(t, hasJSONSchemaKeywords(map[string]interface{}{
		"properties": map[string]interface{}{},
		"type":       "object",
	}))
	// properties alone, no valid type → not detected as schema
	assert.False(t, hasJSONSchemaKeywords(map[string]interface{}{
		"properties": map[string]interface{}{},
	}))
	// properties with non-string type → not detected
	assert.False(t, hasJSONSchemaKeywords(map[string]interface{}{
		"properties": map[string]interface{}{},
		"type":       42,
	}))
}

func TestHasJSONSchemaKeywords_EmptyMap(t *testing.T) {
	assert.False(t, hasJSONSchemaKeywords(map[string]interface{}{}))
}

// ── looksLikePropertyMap ─────────────────────────────────────────────────────

func TestLooksLikePropertyMap(t *testing.T) {
	// empty map – no entries to fail, returns true
	assert.True(t, looksLikePropertyMap(map[string]interface{}{}))

	// all values are objects with a non-empty "type"
	assert.True(t, looksLikePropertyMap(map[string]interface{}{
		"name": map[string]interface{}{"type": "string"},
		"age":  map[string]interface{}{"type": "integer"},
	}))

	// value with a non-empty $ref is acceptable
	assert.True(t, looksLikePropertyMap(map[string]interface{}{
		"nested": map[string]interface{}{"$ref": "#/definitions/Foo"},
	}))

	// value is a primitive, not a map → false
	assert.False(t, looksLikePropertyMap(map[string]interface{}{
		"name": "John",
	}))

	// value is a map but has no "type" and no "$ref" → false
	assert.False(t, looksLikePropertyMap(map[string]interface{}{
		"name": map[string]interface{}{"description": "a name"},
	}))

	// value is a map with empty "type" → false
	assert.False(t, looksLikePropertyMap(map[string]interface{}{
		"name": map[string]interface{}{"type": ""},
	}))

	// $ref present but empty string → falls through to type check, which is missing → false
	assert.False(t, looksLikePropertyMap(map[string]interface{}{
		"name": map[string]interface{}{"$ref": ""},
	}))
}

// ── normalizeLegacyRefs ──────────────────────────────────────────────────────

func TestNormalizeLegacyRefs_RefReplacedWithObject(t *testing.T) {
	input := map[string]interface{}{
		"foo": map[string]interface{}{"$ref": "#/definitions/Foo"},
	}
	result := normalizeLegacyRefs(input)
	assert.Equal(t, map[string]interface{}{"type": "object"}, result["foo"])
}

func TestNormalizeLegacyRefs_ArrayItemsRefReplaced(t *testing.T) {
	input := map[string]interface{}{
		"tags": map[string]interface{}{
			"type":  "array",
			"items": map[string]interface{}{"$ref": "#/definitions/Tag"},
		},
	}
	result := normalizeLegacyRefs(input)
	assert.Equal(t, map[string]interface{}{
		"type":  "array",
		"items": map[string]interface{}{"type": "object"},
	}, result["tags"])
}

func TestNormalizeLegacyRefs_PlainTypeKept(t *testing.T) {
	input := map[string]interface{}{
		"name": map[string]interface{}{"type": "string"},
	}
	result := normalizeLegacyRefs(input)
	assert.Equal(t, map[string]interface{}{"type": "string"}, result["name"])
}

func TestNormalizeLegacyRefs_NonMapValueKept(t *testing.T) {
	input := map[string]interface{}{
		"count": 42,
	}
	result := normalizeLegacyRefs(input)
	assert.Equal(t, 42, result["count"])
}

func TestNormalizeLegacyRefs_ArrayWithoutItemsRefKept(t *testing.T) {
	input := map[string]interface{}{
		"tags": map[string]interface{}{
			"type":  "array",
			"items": map[string]interface{}{"type": "string"},
		},
	}
	result := normalizeLegacyRefs(input)
	assert.Equal(t, map[string]interface{}{
		"type":  "array",
		"items": map[string]interface{}{"type": "string"},
	}, result["tags"])
}

func TestNormalizeLegacyRefs_EmptyRefIgnored(t *testing.T) {
	// $ref present but empty → not treated as a ref, kept as-is
	input := map[string]interface{}{
		"foo": map[string]interface{}{"$ref": "", "type": "string"},
	}
	result := normalizeLegacyRefs(input)
	assert.Equal(t, map[string]interface{}{"$ref": "", "type": "string"}, result["foo"])
}

// ── normalizeSingleSchemaValue ───────────────────────────────────────────────

func TestNormalizeSingleSchemaValue_OwnerTypeNotString(t *testing.T) {
	schemaRep := map[string]interface{}{"type": "json", "value": `{"name":{"type":"string"}}`}
	normalizeSingleSchemaValue(schemaRep, 123)
	assert.Equal(t, `{"name":{"type":"string"}}`, schemaRep["value"])
}

func TestNormalizeSingleSchemaValue_OwnerTypeNotObject(t *testing.T) {
	schemaRep := map[string]interface{}{"type": "json", "value": `{"name":{"type":"string"}}`}
	normalizeSingleSchemaValue(schemaRep, "string")
	assert.Equal(t, `{"name":{"type":"string"}}`, schemaRep["value"])
}

func TestNormalizeSingleSchemaValue_SchemaTypeNotJSON(t *testing.T) {
	schemaRep := map[string]interface{}{"type": "xml", "value": `{"name":{"type":"string"}}`}
	normalizeSingleSchemaValue(schemaRep, "object")
	assert.Equal(t, `{"name":{"type":"string"}}`, schemaRep["value"])
}

func TestNormalizeSingleSchemaValue_EmptyValue(t *testing.T) {
	schemaRep := map[string]interface{}{"type": "json", "value": ""}
	normalizeSingleSchemaValue(schemaRep, "object")
	assert.Equal(t, "", schemaRep["value"])
}

func TestNormalizeSingleSchemaValue_InvalidJSONValue(t *testing.T) {
	schemaRep := map[string]interface{}{"type": "json", "value": "not-valid-json"}
	normalizeSingleSchemaValue(schemaRep, "object")
	assert.Equal(t, "not-valid-json", schemaRep["value"])
}

func TestNormalizeSingleSchemaValue_EmptyObject(t *testing.T) {
	schemaRep := map[string]interface{}{"type": "json", "value": `{}`}
	normalizeSingleSchemaValue(schemaRep, "object")
	assert.Equal(t, `{}`, schemaRep["value"])
}

func TestNormalizeSingleSchemaValue_AlreadyHasSchemaKeywords(t *testing.T) {
	original := `{"type":"object","properties":{"name":{"type":"string"}}}`
	schemaRep := map[string]interface{}{"type": "json", "value": original}
	normalizeSingleSchemaValue(schemaRep, "object")
	// should not be modified – already a valid JSON schema
	assert.Equal(t, original, schemaRep["value"])
}

func TestNormalizeSingleSchemaValue_DoesNotLookLikePropertyMap(t *testing.T) {
	// top-level values are primitives, not property objects
	original := `{"name":"John","age":30}`
	schemaRep := map[string]interface{}{"type": "json", "value": original}
	normalizeSingleSchemaValue(schemaRep, "object")
	assert.Equal(t, original, schemaRep["value"])
}

func TestNormalizeSingleSchemaValue_LegacyPropertyMapWrapped(t *testing.T) {
	schemaRep := map[string]interface{}{
		"type":  "json",
		"value": `{"name":{"type":"string"},"age":{"type":"integer"}}`,
	}
	normalizeSingleSchemaValue(schemaRep, "object")

	normalized, ok := schemaRep["value"].(string)
	require.True(t, ok)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(normalized), &result))
	assert.Equal(t, "object", result["type"])

	props, ok := result["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, props, "name")
	assert.Contains(t, props, "age")
}

func TestNormalizeSingleSchemaValue_LegacyRefInPropertyMap(t *testing.T) {
	// A legacy property map with a $ref; normalizeLegacyRefs should replace it with {"type":"object"}
	schemaRep := map[string]interface{}{
		"type":  "json",
		"value": `{"child":{"$ref":"#/definitions/Child"}}`,
	}
	normalizeSingleSchemaValue(schemaRep, "object")

	normalized, ok := schemaRep["value"].(string)
	require.True(t, ok)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(normalized), &result))
	assert.Equal(t, "object", result["type"])

	props, ok := result["properties"].(map[string]interface{})
	require.True(t, ok)
	childProp, ok := props["child"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "object", childProp["type"])
	assert.NotContains(t, childProp, "$ref")
}

// ── normalizeLegacySchemaValues (integration) ────────────────────────────────

func TestNormalizeLegacySchemaValues_InvalidJSON(t *testing.T) {
	input := []byte("not-valid-json")
	assert.Equal(t, input, normalizeLegacySchemaValues(input))
}

func TestNormalizeLegacySchemaValues_NoSchemaFields(t *testing.T) {
	input := []byte(`{"name":"test","version":1}`)
	result := normalizeLegacySchemaValues(input)
	assert.JSONEq(t, `{"name":"test","version":1}`, string(result))
}

func TestNormalizeLegacySchemaValues_LegacySchemaInNestedActivity(t *testing.T) {
	legacyFlow := `{
		"activities": [{
			"type": "object",
			"schema": {
				"type": "json",
				"value": "{\"name\":{\"type\":\"string\"},\"count\":{\"type\":\"integer\"}}"
			}
		}]
	}`

	result := normalizeLegacySchemaValues([]byte(legacyFlow))

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &parsed))

	activity := parsed["activities"].([]interface{})[0].(map[string]interface{})
	schema := activity["schema"].(map[string]interface{})
	normalizedValue := schema["value"].(string)

	var schemaObj map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(normalizedValue), &schemaObj))

	assert.Equal(t, "object", schemaObj["type"])
	props, ok := schemaObj["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, props, "name")
	assert.Contains(t, props, "count")
}

func TestNormalizeLegacySchemaValues_AlreadyNormalizedSchemaUnchanged(t *testing.T) {
	flow := `{
		"type": "object",
		"schema": {
			"type": "json",
			"value": "{\"type\":\"object\",\"properties\":{\"name\":{\"type\":\"string\"}}}"
		}
	}`

	result := normalizeLegacySchemaValues([]byte(flow))

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &parsed))

	schema := parsed["schema"].(map[string]interface{})
	normalizedValue := schema["value"].(string)

	// The schema already had keywords, so normalizeSingleSchemaValue must have been a no-op.
	assert.JSONEq(t, `{"type":"object","properties":{"name":{"type":"string"}}}`, normalizedValue)
}

func TestNormalizeLegacySchemaValues_NonObjectOwnerTypeSkipped(t *testing.T) {
	// schema present but owner type is "string", not "object" → no normalization
	flow := `{
		"type": "string",
		"schema": {
			"type": "json",
			"value": "{\"name\":{\"type\":\"string\"}}"
		}
	}`

	result := normalizeLegacySchemaValues([]byte(flow))

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &parsed))
	schema := parsed["schema"].(map[string]interface{})
	assert.Equal(t, `{"name":{"type":"string"}}`, schema["value"])
}

func TestNormalizeLegacySchemaValues_ArraysTraversed(t *testing.T) {
	// Verify that arrays at multiple levels are walked correctly.
	flow := `{
		"tasks": [
			{
				"type": "object",
				"schema": {
					"type": "json",
					"value": "{\"id\":{\"type\":\"string\"}}"
				}
			},
			{
				"type": "object",
				"schema": {
					"type": "json",
					"value": "{\"score\":{\"type\":\"number\"}}"
				}
			}
		]
	}`

	result := normalizeLegacySchemaValues([]byte(flow))

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &parsed))

	tasks := parsed["tasks"].([]interface{})
	for i, raw := range tasks {
		task := raw.(map[string]interface{})
		schema := task["schema"].(map[string]interface{})
		var schemaObj map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(schema["value"].(string)), &schemaObj), "task %d", i)
		assert.Equal(t, "object", schemaObj["type"], "task %d should be wrapped", i)
		assert.Contains(t, schemaObj, "properties", "task %d should have properties", i)
	}
}

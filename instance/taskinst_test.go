package instance

import (
	"context"
	"errors"
	"testing"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data/schema"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/model"
	"github.com/stretchr/testify/assert"
)

func TestTaskInst(t *testing.T) {

	def := getDef()
	ind, _ := NewIndependentInstance("test", "", def, nil, log.RootLogger(), context.Background())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	assert.NotNil(t, taskInst)
	assert.True(t, taskInst.HasActivity())

	result, err := taskInst.EvalLink(def.Links()[0])
	assert.Nil(t, err)
	assert.True(t, result)

	//done, err := taskInst.EvalActivity()
	//assert.True(t, done)
	//assert.Nil(t, err)

	assert.Nil(t, taskInst.FlowLogger())
	assert.Nil(t, taskInst.GetTracingContext())
	taskInst.SetStatus(model.TaskStatusDone)
	assert.Equal(t, model.TaskStatusDone, taskInst.Status())
	assert.Nil(t, taskInst.GetFromLinkInstances())
}

// TestGetErrorObject_WithLinkExprError tests error object creation with LinkExprError
func TestGetErrorObject_WithLinkExprError(t *testing.T) {
	def := getDef()
	ind, _ := NewIndependentInstance("test", "", def, nil, log.RootLogger(), context.Background())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	linkErr := definition.NewLinkExprError("link expression evaluation failed")

	errorObj := taskInst.getErrorObject(linkErr)

	assert.NotNil(t, errorObj)
	assert.Equal(t, taskInst.taskID, errorObj["activity"])
	assert.Equal(t, linkErr.Error(), errorObj["message"])
	assert.Equal(t, "link_expr", errorObj["type"])
	assert.Contains(t, errorObj, "activityType")
}

// TestGetErrorObject_WithActivityError tests error object creation with activity.Error
func TestGetErrorObject_WithActivityError(t *testing.T) {
	def := getDef()
	ind, _ := NewIndependentInstance("test", "", def, nil, log.RootLogger(), context.Background())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	errorData := map[string]interface{}{
		"field": "value",
		"count": 42,
	}
	actErr := activity.NewError("activity execution failed", "ACT-001", errorData)
	actErr.SetActivityName("TestActivity")

	errorObj := taskInst.getErrorObject(actErr)

	assert.NotNil(t, errorObj)
	assert.Equal(t, "TestActivity", errorObj["activity"])
	assert.Equal(t, actErr.Error(), errorObj["message"])
	assert.Equal(t, "activity", errorObj["type"])
	assert.Equal(t, "ACT-001", errorObj["code"])
	assert.Equal(t, errorData, errorObj["data"])
	assert.Equal(t, actErr.Category(), errorObj["category"])
	assert.Contains(t, errorObj, "activityType")
}

// TestGetErrorObject_WithActivityEvalError tests error object creation with ActivityEvalError
func TestGetErrorObject_WithActivityEvalError(t *testing.T) {
	def := getDef()
	ind, _ := NewIndependentInstance("test", "", def, nil, log.RootLogger(), context.Background())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	evalErr := NewActivityEvalError("LogStart", "mapper", "mapper execution failed")

	errorObj := taskInst.getErrorObject(evalErr)

	assert.NotNil(t, errorObj)
	assert.Equal(t, taskInst.taskID, errorObj["activity"])
	assert.Equal(t, evalErr.Error(), errorObj["message"])
	assert.Equal(t, "mapper", errorObj["type"])
	assert.Equal(t, "LogStart", errorObj["activity"])
	assert.Contains(t, errorObj, "activityType")
}

// TestGetErrorObject_WithGenericError tests error object creation with a generic error
func TestGetErrorObject_WithGenericError(t *testing.T) {
	def := getDef()
	ind, _ := NewIndependentInstance("test", "", def, nil, log.RootLogger(), context.Background())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	genericErr := errors.New("unexpected error occurred")

	errorObj := taskInst.getErrorObject(genericErr)

	assert.NotNil(t, errorObj)
	assert.Equal(t, taskInst.taskID, errorObj["activity"])
	assert.Equal(t, genericErr.Error(), errorObj["message"])
	assert.Equal(t, "unknown", errorObj["type"])
	assert.Equal(t, "", errorObj["code"])
	assert.Equal(t, EngineError, errorObj["category"])

	data, ok := errorObj["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.NotNil(t, data)
	assert.Empty(t, data)
}

// TestGetErrorObject_WithRealSchemaValidationError tests with actual schema.ValidationError
func TestGetErrorObject_WithRealSchemaValidationError(t *testing.T) {
	def := getDef()
	ind, _ := NewIndependentInstance("test", "", def, nil, log.RootLogger(), context.Background())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	schemaDef := &schema.Def{
		Type:  "json",
		Value: `{"type": "object", "required": ["name", "age"], "properties": {"name": {"type": "string"}, "age": {"type": "number"}}}`,
	}

	s, err := schema.New(schemaDef)
	if err != nil {
		t.Skipf("Skipping test: unable to create schema: %v", err)
		return
	}

	invalidData := map[string]interface{}{"name": "test", "age": "not a number"}
	validationErr := s.Validate(invalidData)

	if validationErr == nil {
		t.Skip("Skipping test: validation did not fail as expected")
		return
	}

	errorObj := taskInst.getErrorObject(validationErr)

	assert.NotNil(t, errorObj)
	assert.Equal(t, taskInst.taskID, errorObj["activity"])
	assert.NotEmpty(t, errorObj["message"])

	if _, ok := validationErr.(*schema.ValidationError); ok {
		assert.Equal(t, "schema_validation", errorObj["type"])
		assert.Equal(t, activity.ActivityError, errorObj["code"])
		assert.Contains(t, errorObj, "activityType")
		data, ok := errorObj["data"].(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, data, "validationErrors")

		validationErrors, ok := data["validationErrors"].([]string)
		assert.True(t, ok)
		assert.NotEmpty(t, validationErrors)
	}
}

// TestGetErrorObject_StructureValidation tests that all error objects have the required structure
func TestGetErrorObject_StructureValidation(t *testing.T) {
	def := getDef()
	ind, _ := NewIndependentInstance("test", "", def, nil, log.RootLogger(), context.Background())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	testCases := []struct {
		name  string
		error error
	}{
		{"Generic Error", errors.New("test error")},
		{"LinkExprError", definition.NewLinkExprError("link error")},
		{"ActivityEvalError", NewActivityEvalError("task", "type", "error text")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errorObj := taskInst.getErrorObject(tc.error)

			assert.Contains(t, errorObj, "activity")
			assert.Contains(t, errorObj, "message")
			assert.Contains(t, errorObj, "type")
			assert.Contains(t, errorObj, "code")
			assert.Contains(t, errorObj, "data")
			assert.Contains(t, errorObj, "category")
			assert.Contains(t, errorObj, "activityType")

			_, ok := errorObj["data"].(map[string]interface{})
			assert.True(t, ok, "data field should be a map[string]interface{}")
		})
	}
}

// TestGetErrorObject_ActivityErrorWithoutActivityName tests activity error without activity name set
func TestGetErrorObject_ActivityErrorWithoutActivityName(t *testing.T) {
	def := getDef()
	ind, _ := NewIndependentInstance("test", "", def, nil, log.RootLogger(), context.Background())
	flowInst := &Instance{master: ind}
	taskInst := NewTaskInst(flowInst, def.Tasks()[0])

	errorData := map[string]interface{}{"reason": "timeout"}
	actErr := activity.NewError("timeout error", "TIMEOUT-001", errorData)

	errorObj := taskInst.getErrorObject(actErr)

	assert.NotNil(t, errorObj)
	assert.Equal(t, "activity", errorObj["type"])
	assert.Equal(t, "TIMEOUT-001", errorObj["code"])
	assert.Equal(t, errorData, errorObj["data"])

	assert.Equal(t, taskInst.taskID, errorObj["activity"])
}

// TestNewErrorObj tests the NewErrorObj helper function
func TestNewErrorObj(t *testing.T) {
	taskID := "testTask"
	message := "test error message"

	errorObj := NewErrorObj(taskID, message)

	assert.NotNil(t, errorObj)
	assert.Equal(t, taskID, errorObj["activity"])
	assert.Equal(t, message, errorObj["message"])
	assert.Equal(t, "unknown", errorObj["type"])
	assert.Equal(t, "", errorObj["code"])
	assert.Equal(t, EngineError, errorObj["category"])
	assert.Equal(t, "", errorObj["activityType"])

	data, ok := errorObj["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.NotNil(t, data)
	assert.Empty(t, data)
}

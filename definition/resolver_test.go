package definition

import (
	"github.com/project-flogo/core/data"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFlowResolver(t *testing.T) {

	scope := data.NewSimpleScope(map[string]interface{}{"key":"value"},nil)
	flowResolver := &FlowResolver{}

	assert.NotNil(t,flowResolver.GetResolverInfo())
	val, err := flowResolver.Resolve(scope,"","key")
	assert.Nil(t, err)
	assert.Equal(t, "value", val.(string))
	_, err = flowResolver.Resolve(scope,"","key")
	assert.Nil(t, err)
}

func TestActivityResolver(t *testing.T) {
	scope := data.NewSimpleScope(map[string]interface{}{"_A.testAct.test":"value", "_A.test2Act":"value"},nil)
	activityResolver := &ActivityResolver{}

	assert.NotNil(t,activityResolver.GetResolverInfo())
	val, err := activityResolver.Resolve(scope, "testAct", "test")
	assert.Nil(t, err)
	assert.Equal(t, "value", val.(string))

	_, err = activityResolver.Resolve(scope, "testAct", "val")
	assert.NotNil(t, err)

	val, err = activityResolver.Resolve(scope, "test2Act", "")
	assert.Nil(t, err)
	assert.Equal(t, "value", val.(string))
}

func TestIteratorResolver(t *testing.T) {
	iterVal := make(map[string]interface{})
	iterVal["test"] = "value"
	scope := data.NewSimpleScope(map[string]interface{}{"_W.iteration":iterVal},nil)

	iteratorResolver := &IteratorResolver{}
	assert.NotNil(t, iteratorResolver.GetResolverInfo())

	val, err := iteratorResolver.Resolve(scope,"test", "")
	assert.Nil(t, err)
	assert.Equal(t, "value", val)
}
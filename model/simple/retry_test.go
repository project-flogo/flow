package simple

//TestTaskContext needs to be enhanced to facilitate these tests

//func TestRetryData(t *testing.T) {
//	taskTestContext := new(TestTaskContext)
//
//
//	rData, err := getRetryData(taskTestContext)
//	assert.NotNil(t, err)
//	assert.Nil(t, rData)
//}
//
//func TestRetryEval(t *testing.T) {
//	taskTestContext := &TestTaskContext{}
//
//	val, err := retryEval(taskTestContext, nil)
//	assert.NotNil(t, err)
//	assert.False(t, val)
//
//	retryData := &RetryData{Interval: 1, Count: 1}
//	val, err = retryEval(taskTestContext, retryData)
//	assert.Nil(t, err)
//	assert.True(t, val)
//	assert.Equal(t, 0, retryData.Count)
//}

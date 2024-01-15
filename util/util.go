package util

import (
	"os"
	"strconv"

	"github.com/mohae/deepcopy"
	"github.com/project-flogo/core/data/coerce"
)

const (
	FlogoStepCount = "FLOGO_STEP_COUNT"
)

func DeepCopy(data interface{}) interface{} {
	return deepcopy.Copy(data)
}

func DeepCopyMap(data map[string]interface{}) map[string]interface{} {
	copiedData := deepcopy.Copy(data)
	copiedMap, _ := coerce.ToObject(copiedData)
	return copiedMap
}

// GetMaxStepCount returns the step limit
func GetMaxStepCount() int64 {
	var maxStepCount int64 = 10000000
	envStepCount, exists := os.LookupEnv(FlogoStepCount)
	if exists {
		i, err := strconv.ParseInt(envStepCount, 10, 64)
		if err == nil {
			return i
		}
	}
	return maxStepCount
}

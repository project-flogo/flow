package util

import (
	"os"
	"strconv"

	"github.com/mohae/deepcopy"
	"github.com/project-flogo/core/data/coerce"
)

const (
	FlogoStepCountEnv   = "FLOGO_MAX_STEP_COUNT"
	MaxStepCountDefault = 10000000
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
func GetMaxStepCount() int {
	envStepCount, exists := os.LookupEnv(FlogoStepCountEnv)
	if !exists {
		return MaxStepCountDefault
	}

	if i, err := strconv.Atoi(envStepCount); err == nil {
		return i
	}

	return MaxStepCountDefault
}

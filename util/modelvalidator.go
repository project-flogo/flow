package util

var modelValidators = make(map[string]ModelValidator)

func RegisterModelValidator(modelName string, validator ModelValidator) {
	modelValidators[modelName] = validator
}

type ModelValidator interface {
	IsValidTaskType(taskType string) bool
}

func GetModelValidator(modelName string) ModelValidator {
	return modelValidators[modelName]
}

func IsValidTaskType(modelName string, taskType string) bool {

	validator, ok := modelValidators[modelName]

	if !ok {
		return false
	}

	return validator.IsValidTaskType(taskType)
}

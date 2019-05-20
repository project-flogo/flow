package model

import (
	"errors"
	"sync"

	"github.com/project-flogo/flow/util"
)

var (
	modelsMu     sync.RWMutex
	models       = make(map[string]*FlowModel)
	defaultModel *FlowModel
)

// Register registers the specified flow model
func Register(flowModel *FlowModel) {
	modelsMu.Lock()
	defer modelsMu.Unlock()

	if flowModel == nil {
		panic("model.Register: model cannot be nil")
	}

	id := flowModel.Name()

	if _, dup := models[id]; dup {
		panic("model.Register: model " + id + " already registered")
	}

	models[id] = flowModel
	util.RegisterModelValidator(id, flowModel)
}

// Registered gets all the registered flow models
func Registered() []*FlowModel {

	modelsMu.RLock()
	defer modelsMu.RUnlock()

	list := make([]*FlowModel, 0, len(models))

	for _, value := range models {
		list = append(list, value)
	}

	return list
}

// Get gets specified FlowModel
func Get(id string) (*FlowModel, error) {
	if _, ok := models[id]; !ok {
		return nil, errors.New("model not found")
	}
	return models[id], nil
}

// Register registers the specified flow model
func RegisterDefault(flowModel *FlowModel) {
	modelsMu.Lock()
	defer modelsMu.Unlock()

	if flowModel == nil {
		panic("model.RegisterDefault: model cannot be nil")
	}

	id := flowModel.Name()

	if _, dup := models[id]; !dup {
		models[id] = flowModel
	}

	defaultModel = flowModel
	util.RegisterModelValidator("", flowModel)
}

func Default() *FlowModel {
	return defaultModel
}

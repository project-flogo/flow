package simple

import (
	"github.com/project-flogo/core/support/logger"
	"github.com/project-flogo/flow/model"
)

// log is the default package logger
var log = logger.GetLogger("flowmodel-simple")

const (
	MODEL_NAME = "flogo-simple"
)

func init() {
	model.Register(New())
}

func New() *model.FlowModel {
	m := model.New(MODEL_NAME)
	m.RegisterFlowBehavior(&FlowBehavior{})
	m.RegisterDefaultTaskBehavior("basic", &TaskBehavior{})
	m.RegisterTaskBehavior("iterator", &IteratorTaskBehavior{})

	return m
}

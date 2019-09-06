package simple

import (
	"github.com/project-flogo/flow/model"
)

const (
	ModelName = "flogo-simple"
)

func init() {
	model.Register(New())
}

func New() *model.FlowModel {
	m := model.New(ModelName)
	m.RegisterFlowBehavior(&FlowBehavior{})
	m.RegisterDefaultTaskBehavior("basic", &TaskBehavior{})
	m.RegisterTaskBehavior("iterator", &IteratorTaskBehavior{})
	m.RegisterTaskBehavior("doWhile", &DoWhileTaskBehavior{})
	return m
}

package test

import (
	"github.com/project-flogo/flow/model"
	"github.com/project-flogo/flow/model/simple"
)

func init() {
	model.Register(NewTestModel())
}

func NewTestModel() *model.FlowModel {
	m := model.New("test")
	m.RegisterFlowBehavior(&simple.FlowBehavior{})
	m.RegisterDefaultTaskBehavior("basic", &simple.TaskBehavior{})

	return m
}

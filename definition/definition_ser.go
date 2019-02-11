package definition

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/data/resolve"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	flowutil "github.com/project-flogo/flow/util"
)

// DefinitionRep is a serializable representation of a flow Definition
type DefinitionRep struct {
	ExplicitReply bool   `json:"explicitReply"`
	Name          string `json:"name"`
	ModelID       string `json:"model"`

	Metadata   *metadata.IOMetadata `json:"metadata"`
	Attributes []*data.Attribute    `json:"attributes,omitempty"`

	Tasks []*TaskRep `json:"tasks"`
	Links []*LinkRep `json:"links"`

	ErrorHandler *ErrorHandlerRep `json:"errorHandler"`
}

// ErrorHandlerRep is a serializable representation of the error flow
type ErrorHandlerRep struct {
	Tasks []*TaskRep `json:"tasks"`
	Links []*LinkRep `json:"links"`
}

// TaskRep is a serializable representation of a flow task
type TaskRep struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Settings map[string]interface{} `json:"settings"`

	ActivityCfgRep *ActivityConfigRep `json:"activity"`
}

// ActivityConfigRep is a serializable representation of an activity configuration
type ActivityConfigRep struct {
	Ref      string                 `json:"ref"`
	Type     string                 `json:"type"` //an alias to the ref, can be used if imported
	Settings map[string]interface{} `json:"settings"`
	Input    map[string]interface{} `json:"input,omitempty"`
	Output   map[string]interface{} `json:"output,omitempty"`
}

// UnmarshalJSON overrides the default UnmarshalJSON for TaskInst
func (ac *ActivityConfigRep) UnmarshalJSON(d []byte) error {
	ser := &struct {
		Ref      string                 `json:"ref"`
		Type     string                 `json:"type"` //an alias to the ref, can be used if imported
		Settings map[string]interface{} `json:"settings"`
		Input    map[string]interface{} `json:"input,omitempty"`
		Output   map[string]interface{} `json:"output,omitempty"`

		//DEPRECATED
		Mappings *mapper.LegacyMappings `json:"mappings,omitempty"`
	}{}

	if err := json.Unmarshal(d, ser); err != nil {
		return err
	}

	ac.Ref = ser.Ref
	ac.Input = ser.Input
	ac.Output = ser.Output

	act := activity.Get(ac.Ref)
	if act != nil {
		if ac.Output != nil {
			for k, v := range act.Metadata().Output {
				if v != nil && v.Type() == data.TypeComplexObject {
					v, err := data.NewTypedValueWithConversion(data.TypeComplexObject, ac.Output[k])
					if err != nil {
						return fmt.Errorf("convert to complex typed value failed %s", err.Error())
					}
					ac.Output[k] = v
				}
			}
		}

		if ac.Input != nil {
			for k, v := range act.Metadata().Input {
				if v != nil && v.Type() == data.TypeComplexObject {
					v, err := data.NewTypedValueWithConversion(data.TypeComplexObject, ac.Input[k])
					if err != nil {
						return fmt.Errorf("convert to complex typed value failed %s", err.Error())
					}
					ac.Input[k] = v
				}
			}
		}
	}

	ac.Type = ser.Type
	ac.Settings = ser.Settings

	if ac.Settings == nil {
		ac.Settings = make(map[string]interface{}, 0)
	}

	input, output, err := mapper.ConvertLegacyMappings(ser.Mappings)
	if err != nil {
		return err
	}

	if ac.Input == nil {
		ac.Input = input
	} else {
		for key, value := range input {
			ac.Input[key] = value
		}
	}

	if ac.Output == nil {
		ac.Output = output
	} else {
		for key, value := range output {
			ac.Output[key] = value
		}
	}

	return nil
}

// LinkRep is a serializable representation of a flow LinkOld
type LinkRep struct {
	Type string `json:"type"`

	Name   string `json:"name"`
	ToID   string `json:"to"`
	FromID string `json:"from"`
	Value  string `json:"value"`
}

// NewDefinition creates a flow Definition from a serializable
// definition representation
func NewDefinition(rep *DefinitionRep) (def *Definition, err error) {

	defer support.HandlePanic("NewDefinition", &err)

	ef := expression.NewFactory(resolve.GetBasicResolver())

	def = &Definition{}
	def.name = rep.Name
	def.modelID = rep.ModelID
	def.metadata = rep.Metadata
	def.explicitReply = rep.ExplicitReply
	if len(rep.Attributes) > 0 {
		def.attrs = make(map[string]*data.Attribute, len(rep.Attributes))

		for _, value := range rep.Attributes {
			def.attrs[value.Name()] = value
		}
	}

	def.tasks = make(map[string]*Task)
	def.links = make(map[int]*Link)

	if len(rep.Tasks) != 0 {

		for _, taskRep := range rep.Tasks {

			task, err := createTask(def, taskRep, ef)

			if err != nil {
				return nil, err
			}
			def.tasks[task.id] = task
		}
	}

	if len(rep.Links) != 0 {

		for id, linkRep := range rep.Links {

			link, err := createLink(def.tasks, linkRep, id, ef)
			if err != nil {
				return nil, err
			}

			def.links[link.id] = link
		}
	}

	if rep.ErrorHandler != nil {

		errorHandler := &ErrorHandler{}
		errorHandler.tasks = make(map[string]*Task)
		errorHandler.links = make(map[int]*Link)
		def.errorHandler = errorHandler

		if len(rep.ErrorHandler.Tasks) != 0 {

			for _, taskRep := range rep.ErrorHandler.Tasks {

				task, err := createTask(def, taskRep, ef)

				if err != nil {
					return nil, err
				}
				errorHandler.tasks[task.id] = task
			}
		}

		if len(rep.ErrorHandler.Links) != 0 {

			idOffset := len(rep.Links)

			for id, linkRep := range rep.ErrorHandler.Links {

				link, err := createLink(errorHandler.tasks, linkRep, id+idOffset, ef)
				if err != nil {
					return nil, err
				}
				errorHandler.links[link.id] = link
			}
		}

	}

	return def, nil
}

func createTask(def *Definition, rep *TaskRep, ef expression.Factory) (*Task, error) {
	task := &Task{}
	task.id = rep.ID
	task.name = rep.Name
	task.definition = def

	if rep.Type != "" {
		if !flowutil.IsValidTaskType(def.modelID, rep.Type) {
			return nil, errors.New("Unsupported task type: " + rep.Type)
		}
		task.typeID = rep.Type
	}

	if len(rep.Settings) > 0 {
		task.settings = make(map[string]interface{}, len(rep.Settings))
		for name, value := range rep.Settings {
			task.settings[name], _ = metadata.ResolveSettingValue(name, value, nil, ef)
		}
	}

	if rep.ActivityCfgRep != nil {

		actCfg, err := createActivityConfig(task, rep.ActivityCfgRep, ef)

		if err != nil {
			return nil, err
		}

		if (actCfg.Details != nil && (actCfg.Details.IsReturn || actCfg.Details.IsReply)) || def.explicitReply {
			def.explicitReply = true
		}

		task.activityCfg = actCfg
	}

	return task, nil
}

func createActivityConfig(task *Task, rep *ActivityConfigRep, ef expression.Factory) (*ActivityConfig, error) {

	if rep.Ref == "" {
		var ok bool
		rep.Ref, ok = support.GetAliasRef("activity", rep.Type)
		if !ok {
			return nil, fmt.Errorf("Activity type '%s' not registered", rep.Type)
		}
	}

	if rep.Ref == "" {
		return nil, errors.New("Activity Not Specified for Task :" + task.ID())
	}

	act := activity.Get(rep.Ref)
	if act == nil {
		return nil, errors.New("Unsupported Activity:" + rep.Ref)
	}

	activityCfg := &ActivityConfig{}
	activityCfg.Activity = act
	activityCfg.Logger = activity.GetLogger(rep.Ref)

	if hasDetails, ok := act.(activity.HasDetails); ok {
		activityCfg.Details = hasDetails.Details()
	}

	//todo need to fix this
	task.activityCfg = activityCfg

	if len(rep.Settings) > 0 {
		activityCfg.settings = make(map[string]interface{}, len(rep.Settings))

		var err error
		mdSettings := act.Metadata().Settings
		for name, value := range rep.Settings {
			activityCfg.settings[name], err = metadata.ResolveSettingValue(name, value, mdSettings, ef)
			if err != nil {
				return nil, err
			}
		}
	}

	mf := GetMapperFactory()

	f := activity.GetFactory(rep.Ref)
	if f != nil {
		ctx := &initCtxImpl{settings: activityCfg.settings, mapperFactory: mf, logger: activity.GetLogger(rep.Ref)}
		var err error
		activityCfg.Activity, err = f(ctx)
		if err != nil {
			return nil, err
		}
	}

	var err error
	activityCfg.inputMapper, err = mf.NewMapper(rep.Input)
	if err != nil {
		return nil, err
	}

	activityCfg.outputMapper, err = mf.NewMapper(rep.Output)
	if err != nil {
		return nil, err
	}

	//If outputMapper is null, use default output mapper
	if activityCfg.outputMapper == nil {
		activityCfg.outputMapper = NewDefaultActivityOutputMapper(task)
	}

	return activityCfg, nil
}

func createLink(tasks map[string]*Task, linkRep *LinkRep, id int, ef expression.Factory) (*Link, error) {

	link := &Link{}
	link.id = id
	link.linkType = LtDependency
	var err error
	if len(linkRep.Type) > 0 {
		switch linkRep.Type {
		case "default", "dependency", "0":
			link.linkType = LtDependency
		case "expression", "1":
			link.linkType = LtExpression
			
			if linkRep.Value == ""{
				return nil, errors.New("Expression value not set")
			}
			link.expr , err = ef.NewExpr(linkRep.Value)
			if err != nil {
				return nil, err
			}
		case "label", "2":
			link.linkType = LtLabel
		case "error", "3":
			link.linkType = LtError
		default:
			//todo get the flow logger
			log.RootLogger().Warnf("Unsupported link type '%s', using default link")
		}
	}

	link.value = linkRep.Value
	link.fromTask = tasks[linkRep.FromID]
	link.toTask = tasks[linkRep.ToID]

	if link.toTask == nil {
		strId := strconv.Itoa(link.ID())
		return nil, errors.New("Link[" + strId + "]: ToTask '" + linkRep.ToID + "' not found")
	}

	if link.fromTask == nil {
		strId := strconv.Itoa(link.ID())
		return nil, errors.New("Link[" + strId + "]: FromTask '" + linkRep.FromID + "' not found")
	}

	// add this link as predecessor "fromLink" to the "toTask"
	link.toTask.fromLinks = append(link.toTask.fromLinks, link)

	// add this link as successor "toLink" to the "fromTask"
	link.fromTask.toLinks = append(link.fromTask.toLinks, link)

	return link, nil
}

type initCtxImpl struct {
	settings      map[string]interface{}
	mapperFactory mapper.Factory
	logger        log.Logger
}

func (ctx *initCtxImpl) Settings() map[string]interface{} {
	return ctx.settings
}

func (ctx *initCtxImpl) MapperFactory() mapper.Factory {
	return ctx.mapperFactory
}

func (ctx *initCtxImpl) Logger() log.Logger {
	return ctx.logger
}

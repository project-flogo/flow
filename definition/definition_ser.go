package definition

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/data/resolve"
	"github.com/project-flogo/core/data/schema"
	"github.com/project-flogo/core/support"
	"github.com/project-flogo/core/support/log"
	flowutil "github.com/project-flogo/flow/util"
)

// DefinitionRep is a serializable representation of a flow Definition
type DefinitionRep struct {
	ExplicitReply bool                 `json:"explicitReply,omitempty"`
	Name          string               `json:"name"`
	ModelID       string               `json:"model,omitempty"`
	Metadata      *metadata.IOMetadata `json:"metadata,omitempty"`
	Tasks         []*TaskRep           `json:"tasks"`
	Links         []*LinkRep           `json:"links,omitempty"`
	ErrorHandler  *ErrorHandlerRep     `json:"errorHandler,omitempty"`
}

// ErrorHandlerRep is a serializable representation of the error flow
type ErrorHandlerRep struct {
	Tasks []*TaskRep `json:"tasks"`
	Links []*LinkRep `json:"links,omitempty"`
}

// TaskRep is a serializable representation of a flow task
type TaskRep struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type,omitempty"`
	Name           string                 `json:"name,omitempty"`
	Settings       map[string]interface{} `json:"settings,omitempty"`
	ActivityCfgRep *ActivityConfigRep     `json:"activity"`
}

// ActivityConfigRep is a serializable representation of an activity configuration
type ActivityConfigRep struct {
	Ref      string                 `json:"ref"`
	Settings map[string]interface{} `json:"settings,omitempty"`
	Input    map[string]interface{} `json:"input,omitempty"`
	Output   map[string]interface{} `json:"output,omitempty"`
	Schemas  *ActivitySchemasRep    `json:"schemas,omitempty"`

	//DEPRECATED
	Type string `json:"type,omitempty"`
}

type ActivitySchemasRep struct {
	Input  map[string]*schema.Def `json:"input,omitempty"`
	Output map[string]*schema.Def `json:"output,omitempty"`
}

// LinkRep is a serializable representation of a flow LinkOld
type LinkRep struct {
	Type   string `json:"type,omitempty"`
	Name   string `json:"name,omitempty"`
	ToID   string `json:"to"`
	FromID string `json:"from"`
	Value  string `json:"value,omitempty"`
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

	mf := GetMapperFactory()

	var err error
	task.settingsMapper, err = mf.NewMapper(rep.Settings)
	if err != nil {
		return nil, err
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

	if rep.Ref == "" && rep.Type != "" {
		log.RootLogger().Warnf("activity configuration 'type' deprecated, use 'ref' in the future")
		rep.Ref = "#" + rep.Type
	}

	if rep.Ref == "" {
		return nil, fmt.Errorf("activity ref not specified for task: %s", task.ID())
	}

	ref := rep.Ref

	if rep.Ref[0] == '#' {
		var ok bool
		ref, ok = support.GetAliasRef("activity", rep.Ref)
		if !ok {
			return nil, fmt.Errorf("activity '%s' not imported", rep.Ref)
		}
	}

	act := activity.Get(ref)
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

	if len(rep.Output) > 0 {
		activityCfg.outputMapper, err = mf.NewMapper(rep.Output)
		if err != nil {
			return nil, err
		}
	}

	//If outputMapper is null, use default output mapper
	if activityCfg.outputMapper == nil {
		activityCfg.outputMapper = NewDefaultActivityOutputMapper(task)
	}

	//schemas

	if rep.Schemas != nil {
		if in := rep.Schemas.Input; in != nil {
			activityCfg.inputSchemas = make(map[string]schema.Schema, len(in))
			for name, def := range in {
				s, err := schema.New(def)
				if err != nil {
					return nil, err
				}
				activityCfg.inputSchemas[name] = s
			}
		}

		if out := rep.Schemas.Output; out != nil {
			activityCfg.outputSchemas = make(map[string]schema.Schema, len(out))
			for name, def := range out {
				s, err := schema.New(def)
				if err != nil {
					return nil, err
				}
				activityCfg.outputSchemas[name] = s
			}
		}
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

			if linkRep.Value == "" {
				return nil, errors.New("expression value not set")
			}
			link.expr, err = ef.NewExpr(linkRep.Value)
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

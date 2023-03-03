package definition

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/project-flogo/core/app/resolve"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
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
	ActivityCfgRep *activity.Config       `json:"activity"`
}

// LinkRep is a serializable representation of a flow LinkOld
type LinkRep struct {
	Type   string `json:"type,omitempty"`
	Name   string `json:"name,omitempty"`
	ToID   string `json:"to"`
	FromID string `json:"from"`
	Label  string `json:"label"`
	Value  string `json:"value,omitempty"`
}

// NewDefinition creates a flow Definition from a serializable
// definition representation
func NewDefinition(rep *DefinitionRep) (def *Definition, err error) {

	defer support.HandlePanic("NewDefinition", &err)

	ef := expression.NewFactory(GetDataResolver())

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
				return nil, fmt.Errorf("error creating task [%s] in flow [%s]: %s", taskRep.ID, rep.Name, err.Error())
			}
			def.tasks[task.id] = task
		}
	}

	if len(rep.Links) != 0 {

		for id, linkRep := range rep.Links {

			link, err := createLink(def.tasks, linkRep, id, ef)
			if err != nil {
				var linkLabel string
				if len(linkRep.Label) > 0 {
					linkLabel = fmt.Sprintf("[%s -> %s] with label [%s]", linkRep.FromID, linkRep.ToID, linkRep.Label)
				} else {
					linkLabel = fmt.Sprintf("[%s -> %s]", linkRep.FromID, linkRep.ToID)
				}
				return nil, fmt.Errorf("error creating link %s in flow [%s]: %s", linkLabel, rep.Name, err.Error())

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
					return nil, fmt.Errorf("error creating task [%s] in flow [%s]'s error handler:%s", taskRep.ID, rep.Name, err.Error())
				}
				errorHandler.tasks[task.id] = task
			}
		}

		if len(rep.ErrorHandler.Links) != 0 {

			idOffset := len(rep.Links)

			for id, linkRep := range rep.ErrorHandler.Links {

				link, err := createLink(errorHandler.tasks, linkRep, id+idOffset, ef)
				if err != nil {
					return nil, fmt.Errorf("error creating link [%s] in flow [%s]'s error handler:%s", linkRep.Name, rep.Name, err.Error())
				}
				errorHandler.links[link.id] = link
			}
		}

	}

	return def, nil
}

func createTask(def *Definition, rep *TaskRep, ef expression.Factory) (*Task, error) {
	var err error
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

	task.loopCfg, err = getLoopCfg(rep.Settings, task.typeID, ef)
	if err != nil {
		return nil, err
	}

	task.retryOnErrConfig, err = getRetryOnErrCfg(rep.Settings, ef)
	if err != nil {
		return nil, err
	}

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

func createActivityConfig(task *Task, rep *activity.Config, ef expression.Factory) (*ActivityConfig, error) {

	if rep.Ref == "" && rep.Type != "" {
		log.RootLogger().Warnf("activity configuration 'type' deprecated, use 'ref' in the future")
		rep.Ref = "#" + rep.Type
	}

	if rep.Ref == "" {
		return nil, fmt.Errorf("activity ref not specified for task: %s", task.ID())
	}

	if rep.Ref[0] == '#' {
		var ok bool
		activityRef := rep.Ref
		rep.Ref, ok = support.GetAliasRef("activity", activityRef)
		if !ok {
			return nil, fmt.Errorf("ref alias '%s' has no corresponding installed activity", activityRef)
		}
	}

	act := activity.Get(rep.Ref)
	if act == nil {
		return nil, fmt.Errorf("unable to find activity with ref [%s]", rep.Ref)
	}

	activityCfg := &ActivityConfig{}
	activityCfg.Activity = act
	activityCfg.HostName = task.definition.name
	activityCfg.Name = task.Name()
	activityCfg.Logger = activity.GetLogger(rep.Ref)
	activityCfg.IsLegacy = activity.HasLegacyActivities() && activity.IsLegacyActivity(rep.Ref)

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
				return nil, fmt.Errorf("unable to resolve setting [%s]'s value [%s]:%s", name, value, err.Error())
			}
		}
	}

	mf := GetMapperFactory()

	f := activity.GetFactory(rep.Ref)
	if f != nil {
		ctx := &initCtxImpl{settings: activityCfg.settings, mapperFactory: mf, logger: activity.GetLogger(rep.Ref), name: task.name,
			hostName: task.definition.name}
		var err error
		activityCfg.Activity, err = f(ctx)
		if err != nil {
			return nil, err
		}
	}

	var err error
	//Convert to correct datatype for input
	input := make(map[string]interface{})
	for k, v := range rep.Input {
		if !isExpr(v) {
			fieldMetaddata, ok := act.Metadata().Input[k]
			if ok {
				newVal, err := coerce.ToType(v, fieldMetaddata.Type())
				if err != nil {
					if fieldMetaddata.Type() == data.TypeConnection {
						connObj, ok := v.(map[string]interface{})
						if ok {
							_, idExist := connObj["id"]
							_, typeExist := connObj["type"]
							if idExist && typeExist {
								//Backward compatible
								input[k] = v
								continue
							}
						}
					}
					if os.Getenv("TEST_MODE") != "true" {
						return nil, fmt.Errorf("unable to convert input [%s]'s value [%s] to type [%s]:%s", k, v, fieldMetaddata.Type(), err.Error())
					} else {
						fmt.Printf("Activity initialization failed for %s \n", activityCfg.Ref())
					}
				}
				input[k] = newVal
			} else {
				//For the cases that metadata comes from iometadata, eg: subflow
				input[k] = v
			}
		} else {
			input[k] = v
		}

	}

	activityCfg.inputMapper, err = mf.NewMapper(input)
	if err != nil {
		return nil, err
	}

	output := make(map[string]interface{})
	for k, v := range rep.Output {
		if !isExpr(v) {
			fieldMetaddata, ok := act.Metadata().Output[k]
			if ok {
				v, err = coerce.ToType(v, fieldMetaddata.Type())
				if err != nil {
					return nil, fmt.Errorf("unable to convert output [%s]'s value [%s] to type [%s]:%s", k, v, fieldMetaddata.Type(), err.Error())
				}
				output[k] = v
			} else {
				output[k] = v
			}

		} else {
			output[k] = v
		}

	}

	if len(rep.Output) > 0 {
		if !activityCfg.IsLegacy {
			//TODO comment out for now. we can reconsider for custom outpout mapper in the future.
			//activityCfg.outputMapper, err = mf.NewMapper(output)
			//if err != nil {
			//	return nil, err
			//}
			activityCfg.outputs = output
		} else {
			activityCfg.outputs = output
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
				s, err := schema.FindOrCreate(def)
				if err != nil {
					return nil, fmt.Errorf("unable to find/create input [%s]'s schema [%+v]:%s", name, def, err.Error())
				}
				activityCfg.inputSchemas[name] = s
			}
		}

		if out := rep.Schemas.Output; out != nil {
			activityCfg.outputSchemas = make(map[string]schema.Schema, len(out))
			for name, def := range out {
				s, err := schema.FindOrCreate(def)
				if err != nil {
					return nil, fmt.Errorf("unable to find/create output [%s]'s schema [%+v]:%s", name, def, err.Error())
				}
				activityCfg.outputSchemas[name] = s
			}
		}
	}

	return activityCfg, nil
}

func isExpr(v interface{}) bool {
	switch t := v.(type) {
	case string:
		if len(t) > 0 && t[0] == '=' {
			return true
		}
	default:
		if ok := mapper.IsConditionalMapping(t); ok {
			return true
		}
		if _, ok := mapper.GetObjectMapping(t); ok {
			return true
		}
	}
	return false
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
				return nil, fmt.Errorf("expression value not set on link")
			}
			link.expr, err = ef.NewExpr(linkRep.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid expression [%s]: %s", linkRep.Value, err.Error())
			}
		case "label", "2":
			link.linkType = LtLabel
		case "error", "3":
			link.linkType = LtError
		case "exprOtherwise", "4":
			link.linkType = LtExprOtherwise
		default:
			//todo get the flow logger
			log.RootLogger().Warnf("Unsupported link type '%s', using default link", linkRep.Type)
		}
	}

	link.value = linkRep.Value
	link.fromTask = tasks[linkRep.FromID]
	link.toTask = tasks[linkRep.ToID]
	link.label = linkRep.Label
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

func getRetryOnErrCfg(settings map[string]interface{}, ef expression.Factory) (RetryOnError, error) {

	retrySetting, ok := settings["retryOnError"]
	if !ok {
		return nil, nil
	}

	retryErr := &retryOnErrConfig{}

	retryCfgMap, err := coerce.ToObject(retrySetting)
	if err != nil {
		return nil, err
	}
	count, exist := retryCfgMap["count"]
	if exist && count != nil {
		strVal, ok := count.(string)
		if ok && len(strVal) > 0 && strVal[0] == '=' {
			if strVal[0] == '=' {
				strVal = strVal[1:]
			}
			conditionExpr, err := ef.NewExpr(strVal)
			if err != nil {
				return nil, fmt.Errorf("compile retry on error condition error: %s", err.Error())
			}
			retryErr.count = conditionExpr
		} else {
			cnt, err := coerce.ToInt(count)
			if err != nil {
				return nil, fmt.Errorf("retryOnError count must be int")
			}
			retryErr.count = cnt
		}

	}

	interval, exist := retryCfgMap["interval"]
	if exist && interval != nil {
		strVal, ok := interval.(string)
		if ok && len(strVal) > 0 && strVal[0] == '=' {
			if strVal[0] == '=' {
				strVal = strVal[1:]
			}
			intervalExpr, err := ef.NewExpr(strVal)
			if err != nil {
				return nil, fmt.Errorf("compile retry on error condition error: %s", err.Error())
			}
			retryErr.interval = intervalExpr
		} else {
			intervalInt, err := coerce.ToInt(interval)
			if err != nil {
				return nil, fmt.Errorf("retryOnError interval must be int")
			}
			retryErr.interval = intervalInt
		}
	}

	return retryErr, nil
}

func getLoopCfg(settings map[string]interface{}, taskType string, ef expression.Factory) (*LoopConfig, error) {

	var loopConfig *LoopConfig
	var err error

	if setting, ok := settings["loopConfig"]; ok {
		loopConfig, err = getLoopCfgDef(setting, ef)
	} else if setting, ok := settings["doWhile"]; ok {
		loopConfig, err = getLoopCfgDef(setting, ef)
		if accum, ok := settings["accumulate"]; loopConfig != nil && ok {
			loopConfig.accumulate, _ = coerce.ToBool(accum)
		}
	} else {
		loopConfig, err = getLoopCfgDef(settings, ef)
	}

	if err != nil {
		return nil, err
	}

	if loopConfig != nil {
		if taskType == "doWhile" {
			loopConfig.accApplyOutput = true
		}
	}
	return loopConfig, nil
}

func getLoopCfgDef(setting interface{}, ef expression.Factory) (*LoopConfig, error) {
	settingMap, err := coerce.ToObject(setting)
	if err != nil {
		return nil, err
	}

	loopConf := &LoopConfig{}
	condition, exist := settingMap["condition"]
	if exist && len(condition.(string)) > 0 {
		conditionStr := condition.(string)
		if conditionStr[0] == '=' {
			conditionStr = conditionStr[1:]
		}
		conditionExpr, err := ef.NewExpr(conditionStr)
		if err != nil {
			return nil, fmt.Errorf("compile loop condition error: %s", err.Error())
		}
		loopConf.condition = conditionExpr
	}

	iterateOn, exist := settingMap["iterateOn"]
	if !exist || iterateOn == nil {
		//Deprecated.
		iterateOn, exist = settingMap["iterate"]
	}

	if exist && iterateOn != nil {
		iteraterOnStr, ok := iterateOn.(string)
		if ok {
			if iteraterOnStr[0] == '=' {
				iteraterOnStr = iteraterOnStr[1:]
			}
			conditionExpr, err := ef.NewExpr(iteraterOnStr)
			if err != nil {
				return nil, fmt.Errorf("compile iterateOn error: %s", err.Error())
			}
			loopConf.iterateOn = conditionExpr
		} else {
			loopConf.iterateOn = iterateOn
		}
	}

	delay, exist := settingMap["delay"]
	if exist && delay != nil {
		strVal, ok := delay.(string)
		if ok && len(strVal) > 0 && strVal[0] == '=' {
			delay, err = resolve.Resolve(strVal[1:], nil)
			if err != nil {
				return nil, err
			}
		}
		delayInt, err := coerce.ToInt(delay)
		if err != nil {
			return nil, fmt.Errorf("loop delay must be int")
		}
		loopConf.delay = delayInt
	}

	accumulateOutput, ok := settingMap["accumulate"]
	if ok {
		loopConf.accumulate, _ = coerce.ToBool(accumulateOutput)
	}

	if loopConf.condition == nil && loopConf.iterateOn == nil {
		return nil, nil
	}
	return loopConf, nil
}

type initCtxImpl struct {
	settings      map[string]interface{}
	mapperFactory mapper.Factory
	logger        log.Logger
	name          string
	hostName      string
}

func (ctx *initCtxImpl) Settings() map[string]interface{} {
	return ctx.settings
}

func (ctx *initCtxImpl) Name() string {
	return ctx.name
}

func (ctx *initCtxImpl) MapperFactory() mapper.Factory {
	return ctx.mapperFactory
}

func (ctx *initCtxImpl) Logger() log.Logger {
	return ctx.logger
}

func (ctx *initCtxImpl) HostName() string {
	return ctx.hostName
}

package instance

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/engine/runner"
	"github.com/project-flogo/core/trigger"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/support"
)

const EventIdAttr = "event.id"

func applySettingsMapper(taskInst *TaskInst) error {

	// get the input mapper
	settingsMapper := taskInst.task.SettingsMapper()

	if settingsMapper != nil {

		taskInst.logger.Debug("Applying SettingsMapper")

		var err error
		taskInst.settings, err = settingsMapper.Apply(taskInst.flowInst)

		if err != nil {
			return err
		}
	}

	return nil
}

func applyInputMapper(taskInst *TaskInst) error {

	// get the input mapper
	inputMapper := taskInst.task.ActivityConfig().InputMapper()

	master := taskInst.flowInst.master

	if master.patch != nil {
		// check if the patch has a overriding mapper
		mapper := master.patch.GetInputMapper(taskInst.task.ID())
		if mapper != nil {
			inputMapper = mapper
		}
	}

	if inputMapper != nil {

		taskInst.logger.Debug("Applying InputMapper")

		var inputScope data.Scope
		inputScope = taskInst.flowInst

		if taskInst.workingData != nil {
			inputScope = taskInst.workingData
		}

		var err error

		taskInst.inputs, err = inputMapper.Apply(inputScope)

		if err != nil {
			return err
		}
	}

	return nil
}

func applyInputInterceptor(taskInst *TaskInst) bool {

	master := taskInst.flowInst.master

	if master.interceptor != nil {

		// check if this task as an interceptor
		taskInterceptor := master.interceptor.GetTaskInterceptor(taskInst.task.ID())
		if taskInterceptor != nil {

			taskInst.logger.Debug("Applying Interceptor - Input")

			if len(taskInterceptor.Inputs) > 0 {
				// override input attributes
				mdInputs := taskInst.task.ActivityConfig().Activity.Metadata().Input
				var err error
				for name, value := range taskInterceptor.Inputs {

					if taskInst.logger.DebugEnabled() {
						taskInst.logger.Debugf("Overriding Input Attr: %s = %s", name, value)
					}

					if taskInst.inputs == nil {
						taskInst.inputs = make(map[string]interface{})
					}
					if mdAttr, ok := mdInputs[name]; ok {
						taskInst.inputs[name], err = coerce.ToType(value, mdAttr.Type())
						if err != nil {
							//handler err
						}
					} else {
						taskInst.inputs[name] = value
					}
				}
			}

			// check if we should not evaluate the task
			return !taskInterceptor.Skip
		}
	}

	return true
}

func applyAssertionInterceptor(taskInst *TaskInst) error {

	master := taskInst.flowInst.master
	if master.interceptor != nil {
		taskInst.logger.Debug("Applying Interceptor - Assertion")
		// check if this task has assertion interceptor
		id := taskInst.flowInst.Name() + "-" + taskInst.task.ID()
		taskInterceptor := master.interceptor.GetTaskInterceptor(id)
		if taskInterceptor != nil && len(taskInterceptor.Assertions) > 0 {
			ef := expression.NewFactory(definition.GetDataResolver())

			for name, assertion := range taskInterceptor.Assertions {
				if taskInst.logger.DebugEnabled() {
					taskInst.logger.Debugf("Executing Assertion Attr: %s = %s", name, assertion)
				}
				result := false
				var message string

				if assertion.Expression == "" {
					taskInterceptor.Assertions[name].Message = "Empty expression"
					taskInterceptor.Assertions[name].Result = support.NotExecuted
					continue
				}

				if assertion.Type == support.Primitive {
					result, message = applyPrimitiveAssertion(taskInst, ef, assertion)
				} else {
					taskInst.Logger().Errorf("Invalid Assertion Mode")
					return errors.New("Invalid Assertion Mode")
				}

				taskInterceptor.Assertions[name].Message = message
				//Set the result back in the Interceptor.
				if result {
					taskInterceptor.Assertions[name].Result = support.Pass
				} else {
					taskInterceptor.Assertions[name].Result = support.Fail
				}
				taskInst.logger.Debugf("Assertion Execution Result => Name: %s, Assertion Expression: %v, Result: %s, Message: %s ",
					assertion.Name, assertion.Expression, strconv.FormatBool(result), message)
			}
		}
	}

	return nil
}

func applyPrimitiveAssertion(taskInst *TaskInst, ef expression.Factory, assertion support.Assertion) (bool, string) {

	expr, _ := ef.NewExpr(fmt.Sprintf("%v", assertion.Expression))
	if expr == nil {
		return false, "Failed to validate expression"
	}

	result, err := expr.Eval(taskInst.flowInst)
	if err != nil {
		taskInst.logger.Error(err)
		return false, "Failed to evaluate expression"
	}
	res, _ := coerce.ToBool(result)

	if res {
		return res, "Comparison success"
	} else {
		return res, "Comparison failure"
	}
}
func hasOutputInterceptor(taskInst *TaskInst) bool {
	master := taskInst.flowInst.master

	if master.interceptor != nil {

		taskInst.logger.Debug("Checking for Interceptor - Output")

		id := taskInst.flowInst.Name() + "-" + taskInst.task.ID()
		taskInterceptor := master.interceptor.GetTaskInterceptor(id)
		if taskInterceptor != nil && taskInterceptor.SkipExecution {
			return true
		}
	}
	return false
}

func applyOutputInterceptor(taskInst *TaskInst) error {

	master := taskInst.flowInst.master

	if master.interceptor != nil {

		taskInst.logger.Debug("Applying Interceptor - Output")

		id := taskInst.flowInst.Name() + "-" + taskInst.task.ID()
		// check if this task as an interceptor and overrides ouputs
		taskInterceptor := master.interceptor.GetTaskInterceptor(id)
		if taskInterceptor != nil && len(taskInterceptor.Outputs) > 0 {

			mdOutput := taskInst.task.ActivityConfig().Activity.Metadata().Output
			var err error

			// override output attributes
			for name, value := range taskInterceptor.Outputs {

				if taskInst.logger.DebugEnabled() {
					taskInst.logger.Debugf("Overriding Output Attr: %s = %s", name, value)
				}

				if taskInst.outputs == nil {
					taskInst.outputs = make(map[string]interface{})
				}
				if mdAttr, ok := mdOutput[name]; ok {
					taskInst.outputs[name], err = coerce.ToType(value, mdAttr.Type())
					if err != nil {
						return err
					}
				} else {
					taskInst.outputs[name] = value
				}
			}
		}
	}

	return nil
}

func setActivityExecutionStatus(taskInst *TaskInst, status int) {
	master := taskInst.flowInst.master

	if master.interceptor != nil {

		taskInst.logger.Debug("Setting activity execution status")
		id := taskInst.flowInst.Name() + "-" + taskInst.task.ID()
		taskInterceptor := master.interceptor.GetTaskInterceptor(id)
		if taskInterceptor != nil {
			taskInterceptor.Result = status
		}
	}
}

func setActivityExecutionMessage(taskInst *TaskInst, message string) {
	master := taskInst.flowInst.master

	if master.interceptor != nil {

		taskInst.logger.Debug("Setting activity execution status")
		id := taskInst.flowInst.Name() + "-" + taskInst.task.ID()
		taskInterceptor := master.interceptor.GetTaskInterceptor(id)
		if taskInterceptor != nil {
			taskInterceptor.Message = message
		}
	}
}

// applyOutputMapper applies the output mapper, returns flag indicating if
// there was an output mapper
func applyOutputMapper(taskInst *TaskInst) (bool, error) {

	// get the Output Mapper for the TaskOld if one exists
	outputMapper := taskInst.task.ActivityConfig().OutputMapper()

	master := taskInst.flowInst.master

	if master.patch != nil {
		// check if the patch overrides the Output Mapper
		mapper := master.patch.GetOutputMapper(taskInst.task.ID())
		if mapper != nil {
			outputMapper = mapper
		}
	}

	if outputMapper != nil {
		taskInst.logger.Debug("Applying OutputMapper")

		values, err := outputMapper.Apply(data.NewSimpleScope(taskInst.outputs, nil))

		for name, value := range values {
			_ = taskInst.flowInst.SetValue(name, value)
			//if taskInst.flowInst.attrs == nil {
			//	taskInst.flowInst.attrs = make(map[string]interface{})
			//}
			//taskInst.flowInst.attrs[name] = value //data.ToTypedValue(value)
		}

		return true, err
	}

	return false, nil
}

func GetFlowIOMetadata(flowURI string) (*metadata.IOMetadata, error) {

	def, _, err := support.GetDefinition(flowURI)
	if err != nil {
		return nil, err
	}
	if def == nil {
		return nil, errors.New("unable to resolve subflow: " + flowURI)
	}

	return def.Metadata(), nil
}

func StartSubFlow(ctx activity.Context, flowURI string, inputs map[string]interface{}) error {

	taskInst, ok := ctx.(*TaskInst)

	if !ok {
		return errors.New("unable to create subFlow using this context")
	}

	def, _, err := support.GetDefinition(flowURI)
	if err != nil {
		return err
	}
	if def == nil {
		return errors.New("unable to resolve subflow: " + flowURI)
	}

	//todo make sure that there is only one subFlow per taskinst
	flowInst := taskInst.flowInst.master.newEmbeddedInstance(taskInst, flowURI, def)

	ctx.Logger().Debugf("starting embedded subflow `%s`", flowInst.Name())

	err = taskInst.flowInst.master.startEmbedded(flowInst, inputs)
	if err != nil {
		return err
	}

	return nil
}

func StartDetachedSubFlow(ctx activity.Context, flowURI string, inputs map[string]interface{}) error {
	taskInst, ok := ctx.(*TaskInst)

	if !ok {
		return errors.New("unable to create subFlow using this context")
	}
	f := action.GetFactory("github.com/project-flogo/flow")
	flowAction, err := f.New(&action.Config{Settings: map[string]interface{}{"flowURI": flowURI}})
	if err != nil {
		return err
	}

	ro := &RunOptions{}
	ro.Op = OpStart
	ro.DetachExecution = true
	inputs["_run_options"] = ro
	eventId, _ := taskInst.flowInst.GetValue(EventIdAttr)
	gCtx := context.Background()
	if eventId != "" {
		gCtx = trigger.NewContextWithEventId(gCtx, eventId.(string))
	}
	_, err = runner.NewDirect().RunAction(gCtx, flowAction, inputs)
	if err != nil {
		return err
	}
	return nil
}

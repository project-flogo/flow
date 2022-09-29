package instance

import (
	"errors"
	"fmt"
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/flow/definition"
	"github.com/project-flogo/flow/support"
	"reflect"
	"strconv"
)

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
					continue
				}

				if assertion.Type == support.Primitive {
					result, message = applyPrimitiveAssertion(taskInst, ef, assertion)
				} else if assertion.Type == support.Activity {
					result = applyActivityAssertion(taskInst, assertion)
				} else {
					taskInst.Logger().Errorf("Invalid Assertion Mode")
				}

				taskInterceptor.Assertions[name].Message = message
				//Set the result back in the Interceptor.
				if result {
					taskInterceptor.Assertions[name].Result = 1
				} else {
					taskInterceptor.Assertions[name].Result = 2
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

	if checkForComparisonExpression(expr) {
		return false, "Not a comparison expression"
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

func checkForComparisonExpression(expr expression.Expr) bool {

	of := reflect.TypeOf(expr)
	switch of.String() {
	case "*ast.cmpEqExpr":
	case "*ast.cmpNotEqExpr":
	case "*ast.cmpLtExpr":
	case "*ast.cmpLtEqExpr":
	case "*ast.cmpGtExpr":
	case "*ast.cmpGtEqExpr":
		return true
	}
	return false
}

func applyActivityAssertion(taskInst *TaskInst, assertion support.Assertion) bool {

	// Gold output will be always be JSON.
	goldOutput := assertion.Expression.(map[string]interface{})
	equal := reflect.DeepEqual(goldOutput, taskInst.outputs)
	return equal
}

func hasOutputInterceptor(taskInst *TaskInst) bool {
	master := taskInst.flowInst.master

	if master.interceptor != nil {

		taskInst.logger.Debug("Checking for Interceptor - Output")

		taskInterceptor := master.interceptor.GetTaskInterceptor(taskInst.task.ID())
		if taskInterceptor != nil && len(taskInterceptor.Outputs) > 0 {
			return true
		}
	}
	return false
}

func applyOutputInterceptor(taskInst *TaskInst) error {

	master := taskInst.flowInst.master

	if master.interceptor != nil {

		taskInst.logger.Debug("Applying Interceptor - Output")

		// check if this task as an interceptor and overrides ouputs
		taskInterceptor := master.interceptor.GetTaskInterceptor(taskInst.task.ID())
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

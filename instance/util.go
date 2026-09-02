package instance

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/project-flogo/core/data/expression/script/gocc/ast"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/expression"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/engine/runner"
	"github.com/project-flogo/core/engine/support"
	"github.com/project-flogo/core/trigger"
	"github.com/project-flogo/flow/definition"
	flowSupport "github.com/project-flogo/flow/support"
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
		// check if the patch has an overriding mapper
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

func applyAssertionInterceptor(taskInst *TaskInst, assertType int) error {

	master := taskInst.flowInst.master
	if master.interceptor != nil {
		taskInst.logger.Debug("Applying Interceptor - Assertion")
		// check if this task has assertion interceptor
		id := taskInst.flowInst.Name() + "-" + taskInst.task.ID()
		taskInterceptor := master.interceptor.GetTaskInterceptor(id)
		if taskInterceptor != nil && len(taskInterceptor.Assertions) > 0 {
			ef := expression.NewFactory(definition.GetDataResolver())

			for name, assertion := range taskInterceptor.Assertions {
				if taskInterceptor.Type != assertType {
					taskInterceptor.Assertions[name].Result = support.AssertionNotExecuted
					continue
				}
				if taskInst.logger.DebugEnabled() {
					taskInst.logger.Debugf("Executing Assertion Attr: %d = %v", name, assertion)
				}
				result := false
				var message string
				var evalData ast.ExprEvalData
				if assertion.Expression == "" {
					taskInterceptor.Assertions[name].Message = "Empty expression"
					taskInterceptor.Assertions[name].Result = support.NotExecuted
					continue
				}

				if assertion.Type == support.Primitive {
					result, message, evalData = applyPrimitiveAssertion(taskInst, ef, &assertion)
				} else {
					taskInst.Logger().Errorf("Invalid Assertion Mode")
					return errors.New("invalid assertion mode")
				}

				taskInterceptor.Assertions[name].Message = message
				taskInterceptor.Assertions[name].EvalResult = evalData
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

func applyPrimitiveAssertion(taskInst *TaskInst, ef expression.Factory, assertion *support.Assertion) (bool, string, ast.ExprEvalData) {

	expr, _ := ef.NewExpr(fmt.Sprintf("%v", assertion.Expression))
	if expr == nil {
		return false, "Failed to validate expression", ast.ExprEvalData{}
	}
	result, err := expr.Eval(taskInst.flowInst)
	if err != nil {
		taskInst.logger.Error(err)
		return false, "Failed to evaluate expression", ast.ExprEvalData{}
	}

	exp, ok := expr.(ast.ExprEvalResult)
	var resultData ast.ExprEvalData
	if ok {
		resultData = exp.Detail()
	}

	res, _ := coerce.ToBool(result)

	if res {
		return res, "Comparison success", resultData
	} else {
		return res, "Comparison failure", resultData
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
		// check if this task as an interceptor and overrides outputs
		taskInterceptor := master.interceptor.GetTaskInterceptor(id)
		if taskInterceptor != nil && len(taskInterceptor.Outputs) > 0 {

			if taskInterceptor.Type == support.MockActivity {
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
			if taskInterceptor.Type == support.MockException {
				message := taskInterceptor.Outputs["message"].(string)
				data := taskInterceptor.Outputs["data"]
				if data == nil {
					data = struct{}{}
				}
				category := activity.ActivityError
				if cat, ok := taskInterceptor.Outputs["category"].(string); ok && cat != "" {
					category = activity.ErrorCategory(cat)
				}
				code := ""
				if c, ok := taskInterceptor.Outputs["code"].(string); ok {
					code = c
				}
				e := activity.NewActivityError(message, code, category, data)
				e.SetActivityName(taskInst.id)
				return e
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

		rootObj := make(map[string]string, len(values))

		for name, value := range values {
			_ = taskInst.flowInst.SetValue(name, value)
		}

		if taskInst.Task().LoopConfig() == nil {
			// If the task is not looping, we store the root object with all the activity output paths
			for name := range values {
				// Add field paths to the root object for resolving $activity[ActivityName] in the mappings
				// This is done to avoid memory overhead of storing the root object with all the activity outputs which are already set in the scope
				// Check github.com/project-flogo/flow/definition/resolve.go#(r *ActivityResolver) Resolve(...)
				rootObj[name] = ""
			}
			_ = taskInst.flowInst.SetValue("_A."+taskInst.id, rootObj)
		}
		return true, err
	}

	return false, nil
}

func GetFlowIOMetadata(flowURI string) (*metadata.IOMetadata, error) {

	def, _, err := flowSupport.GetDefinition(flowURI)
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

	def, _, err := flowSupport.GetDefinition(flowURI)
	if err != nil {
		return err
	}
	if def == nil {
		return errors.New("unable to resolve subflow: " + flowURI)
	}

	var cancelctx context.Context = nil
	var cancelFunc context.CancelFunc = nil
	if taskInst.flowInst.goContext != nil {
		// FLOGO-19484: derive a per-instance cancel rather than ALIASING the parent's.
		//
		// handleTaskDone calls containerInst.cancelFunc() unconditionally when an embedded
		// instance completes. Aliasing therefore meant a nested subflow completing cancelled its
		// PARENT's context, after which every remaining task in the parent hit the ctx.Done()
		// guard in EvalActivity and silently returned without applying its output mapper. That
		// was always wrong; it becomes damaging with transactional subflows, where the parent is
		// still running and expects to reach its own terminal transition.
		//
		// Deriving keeps parent -> child cancellation propagation intact; it only stops the
		// child from cancelling upwards.
		cancelctx, cancelFunc = context.WithCancel(taskInst.flowInst.goContext)
	}
	//defer cancelFunc()
	//todo make sure that there is only one subFlow per taskinst
	flowInst := taskInst.flowInst.master.newEmbeddedInstance(taskInst, flowURI, def, cancelctx, cancelFunc)

	ctx.Logger().Debugf("starting embedded subflow `%s`", flowInst.Name())

	attr, isLoop := taskInst.GetWorkingData("iterateIndex")
	index := ""
	if isLoop {
		index = attr.(string)
	}
	taskInst.flowInst.master.addSubFlowToCoverage(def.Name(), taskInst.Name(), taskInst.flowInst.Name(), taskInst.flowInst.ID(), flowInst.ID(), inputs, isLoop, index)
	err = taskInst.flowInst.master.startEmbedded(flowInst, inputs)
	if err != nil {
		return err
	}

	return nil
}

func StartSubFlowWithContext(duration int64, ctx activity.Context, flowURI string, inputs map[string]interface{}) error {

	taskInst, ok := ctx.(*TaskInst)

	if !ok {
		return errors.New("unable to create subFlow using this context")
	}

	taskInst.logger.Debugf("starting subflow `%s` with timeout %v ", flowURI, duration)
	def, _, err := flowSupport.GetDefinition(flowURI)
	if err != nil {
		return err
	}
	if def == nil {
		return errors.New("unable to resolve subflow: " + flowURI)
	}

	timeout := time.Duration(duration) * time.Millisecond

	timeoutContext, cancelFunc := context.WithTimeout(context.Background(), timeout)
	timeoutContext = context.WithValue(timeoutContext, "timeoutContext", "true")
	timeoutContext = context.WithValue(timeoutContext, "timeoutSeconds", strconv.FormatInt(duration, 10))
	taskInst.logger.Debugf("context %v ", timeoutContext)

	// cancelFunc is deliberately not deferred here: ownership passes to the embedded
	// instance below, which keeps using timeoutContext long after this function returns.
	// The instance calls it from handleTaskDone once the subflow completes
	// (ind_instance.go, "if containerInst.cancelFunc != nil") and from
	// execTaskWithContext on each timeout branch.
	//
	// FLOGO-19484 - transactional subflow: do NOT begin a database transaction on
	// timeoutContext. It is cancelled when the subflow completes, and that happens
	// *before* the host TaskInst is rescheduled, so database/sql's watchdog would roll
	// the transaction back before the subflow activity ever gets to commit it. Begin the
	// transaction on a non-cancellable context and observe cancellation separately.
	//defer cancelFunc()
	//todo make sure that there is only one subFlow per taskinst
	flowInst := taskInst.flowInst.master.newEmbeddedInstance(taskInst, flowURI, def, timeoutContext, cancelFunc)

	ctx.Logger().Debugf("starting embedded subflow `%s`", flowInst.Name())

	attr, isLoop := taskInst.GetWorkingData("iterateIndex")
	index := ""
	if isLoop {
		index = attr.(string)
	}
	taskInst.flowInst.master.addSubFlowToCoverage(def.Name(), taskInst.Name(), taskInst.flowInst.Name(), taskInst.flowInst.ID(), flowInst.ID(), inputs, isLoop, index)
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
	eventIdAttr, ok := taskInst.flowInst.GetValue(EventIdAttr)
	if !ok {
		eventIdAttr, _ = taskInst.flowInst.master.Instance.GetValue(EventIdAttr)
	}

	gCtx := context.Background()
	if eventId, ok := eventIdAttr.(string); ok && eventId != "" {
		gCtx = trigger.NewContextWithEventId(gCtx, eventId)
	}
	_, err = runner.NewDirect().RunAction(gCtx, flowAction, inputs)
	if err != nil {
		return err
	}
	return nil
}

// StartTransactionalSubFlow starts an embedded subflow that runs inside a database transaction
// (FLOGO-19484). It is ADDITIVE: StartSubFlow, StartSubFlowWithContext and StartDetachedSubFlow
// are untouched, so an older build of the separately-versioned flow/activity/subflow module
// still compiles against a newer flow.
//
// The ENGINE owns context construction. The activity supplies only `decorate`, which layers the
// transaction handle onto the context, and `fin`, which commits or rolls back; flow/instance
// never imports database/sql.
func StartTransactionalSubFlow(ctx activity.Context, flowURI string, inputs map[string]interface{},
	timeoutMs int64, connID string, decorate TxContextDecorator, fin TxFinalizer) error {

	taskInst, ok := ctx.(*TaskInst)
	if !ok {
		return errors.New("unable to create subFlow using this context")
	}
	if fin == nil {
		return errors.New("a transactional subflow requires a transaction finalizer")
	}

	def, _, err := flowSupport.GetDefinition(flowURI)
	if err != nil {
		return err
	}
	if def == nil {
		return errors.New("unable to resolve subflow: " + flowURI)
	}

	// The parent is the FLOW's context, NEVER ctx.GoContext().
	//
	// In concurrent mode execTaskConcurrent sets taskInst.evalCtx = inst.concurCtx and
	// TaskInst.GoContext() prefers evalCtx, while RunConcurrent cancels and nils concurCtx at the
	// end of the round - by which time this subflow has only been ENQUEUED. The embedded instance
	// would carry a permanently cancelled context and every task evaluation would silently drop
	// its tail (output mappers, accumulation, _E). StartSubFlow already reads flowInst.goContext.
	parent := taskInst.flowInst.goContext
	if parent == nil {
		parent = context.Background()
	}
	if decorate != nil {
		parent = decorate(parent) // layers the transaction registry
	}

	var cancels []context.CancelFunc
	if timeoutMs != 0 {
		// The same two untyped string keys DoStep and handleTaskCancelled read, so the subflow's
		// tasks route through execTaskWithContext and the execTimeout rollback works - without
		// touching StartSubFlowWithContext's own parentage.
		parent = context.WithValue(parent, "timeoutContext", "true")
		parent = context.WithValue(parent, "timeoutSeconds", strconv.FormatInt(timeoutMs, 10))
		tctx, tcancel := context.WithTimeout(parent, time.Duration(timeoutMs)*time.Millisecond)
		parent, cancels = tctx, append(cancels, tcancel)
	}

	// The embedded instance gets its OWN cancel, never the parent flow's. handleTaskDone calls
	// containerInst.cancelFunc() unconditionally, so aliasing the parent's cancelFunc - what
	// StartSubFlow does - would cancel the PARENT flow when this subflow completes.
	subCtx, subCancel := context.WithCancel(parent)
	cancels = append(cancels, subCancel)
	cancelAll := func() {
		for i := len(cancels) - 1; i >= 0; i-- {
			cancels[i]()
		}
	}
	// cancelAll only ever touches OUR subtree. The transaction is begun by the activity on a
	// context.Background()-rooted context, so none of this can trigger database/sql's automatic
	// rollback before the finalizer runs.

	master := taskInst.flowInst.master
	flowInst := master.newEmbeddedInstance(taskInst, flowURI, def, subCtx, cancelAll)

	// Publish txScope under the state lock. newEmbeddedInstance already put flowInst into
	// master.subflows under that lock, and RollbackOpenTransactions iterates subflows under it,
	// so assigning the field outside the lock would be a read/write race on a field the sweep
	// inspects. No-op in sequential mode.
	scope := &txScope{fin: fin, connID: connID, logger: ctx.Logger()}
	master.lockState()
	flowInst.txScope = scope
	master.unlockState()
	master.txScopeActive.Add(1)

	ctx.Logger().Debugf("FLOGO-19484: starting transactional embedded subflow `%s` on connection '%s'", flowInst.Name(), connID)

	attr, isLoop := taskInst.GetWorkingData("iterateIndex")
	index := ""
	if isLoop {
		index = attr.(string)
	}
	master.addSubFlowToCoverage(def.Name(), taskInst.Name(), taskInst.flowInst.Name(),
		taskInst.flowInst.ID(), flowInst.ID(), inputs, isLoop, index)

	if err = master.startEmbedded(flowInst, inputs); err != nil {
		// Nothing was scheduled and nothing else will ever finalise this scope.
		master.lockState()
		flowInst.txScope = nil
		master.unlockState()
		master.txScopeActive.Add(-1)
		cancelAll()
		return err
	}

	return nil
}

func IsConcurrentTaskExcutionEnabled() bool {
	return flowSupport.GetConcurrentExecution()
}

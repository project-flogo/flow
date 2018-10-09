package test

import (
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/support/logger"
)

var log = logger.GetLogger("test-activity")

func init() {
	activity.Register(NewLogActivity())
	activity.Register(NewCounterActivity())
}

type LogActivity struct {
	metadata *activity.Metadata
}

// NewActivity creates a new AppActivity
func NewLogActivity() activity.Activity {
	metadata := &activity.Metadata{ID: "test-log"}
	input := map[string]*data.Attribute{
		"message": data.NewZeroAttribute("message", data.TypeString),
	}
	metadata.Input = input
	output := map[string]*data.Attribute{
		"message": data.NewZeroAttribute("message", data.TypeString),
	}
	metadata.Output = output
	return &LogActivity{metadata: metadata}
}

// Metadata returns the activity's metadata
func (a *LogActivity) Metadata() *activity.Metadata {
	return a.metadata
}

// Eval implements api.Activity.Eval - Logs the Message
func (a *LogActivity) Eval(context activity.Context) (done bool, err error) {

	log.Debugf("eval test-log activity")

	message, _ := context.GetInput("message").(string)

	log.Infof("message: %s", message)

	context.SetOutput("message", message)

	return true, nil
}

type CounterActivity struct {
	metadata *activity.Metadata
	counters map[string]int
}

// NewActivity creates a new AppActivity
func NewCounterActivity() activity.Activity {
	metadata := &activity.Metadata{ID: "test-counter"}
	input := map[string]*data.Attribute{
		"counterName": data.NewZeroAttribute("counterName", data.TypeString),
	}
	metadata.Input = input
	output := map[string]*data.Attribute{
		"value": data.NewZeroAttribute("value", data.TypeInteger),
	}
	metadata.Output = output
	return &CounterActivity{metadata: metadata, counters: make(map[string]int)}
}

// Metadata returns the activity's metadata
func (a *CounterActivity) Metadata() *activity.Metadata {
	return a.metadata
}

// Eval implements api.Activity.Eval - Logs the Message
func (a *CounterActivity) Eval(context activity.Context) (done bool, err error) {

	log.Debugf("eval test-counter activity")

	counterName, _ := context.GetInput("counterName").(string)

	log.Debugf("counterName: %s", counterName)

	count := 1

	if counter, exists := a.counters[counterName]; exists {
		count = counter + 1
	}

	a.counters[counterName] = count

	log.Debugf("value: %s", count)

	context.SetOutput("value", count)

	return true, nil
}

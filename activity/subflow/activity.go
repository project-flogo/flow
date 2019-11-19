package subflow

import (
	"github.com/project-flogo/core/support/log"
	"sync"
	"sync/atomic"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/flow/instance"
)

func init() {
	_ = activity.Register(&SubFlowActivity{}, New)
}

type Settings struct {
	FlowURI string `md:"flowURI,required"`
}

var activityMd = activity.ToMetadata(&Settings{})

func New(ctx activity.InitContext) (activity.Activity, error) {
	s := &Settings{}
	err := metadata.MapToStruct(ctx.Settings(), s, true)
	if err != nil {
		return nil, err
	}

	//todo make sure we are part of a flow, since this only works in a flow

	//minimal uri check
	//_, err = url.ParseRequestURI(s.FlowURI)
	//if err != nil {
	//	return nil, err
	//}

	activityMd := activity.ToMetadata(&Settings{})
	act := &SubFlowActivity{flowURI: s.FlowURI, activityMd: activityMd}

	ctx.Logger().Debugf("flowURI: %+v", s.FlowURI)

	return act, nil
}

// SubFlowActivity is an Activity that is used to start a sub-flow, can only be used within the
// context of an flow
// settings: {flowURI}
// input : {sub-flow's input}
// output: {sub-flow's output}
type SubFlowActivity struct {
	activityMd *activity.Metadata
	flowURI    string

	mutex     sync.Mutex
	mdUpdated uint32
}

// Metadata returns the activity's metadata
func (a *SubFlowActivity) Metadata() *activity.Metadata {

	if a.activityMd == nil {
		//singleton version of activity
		return activityMd
	}

	// have to lazy init for now, because resources are not loaded based on dependency
	if atomic.LoadUint32(&a.mdUpdated) == 0 {
		a.mutex.Lock()
		defer a.mutex.Unlock()
		if a.mdUpdated == 0 {
			flowIOMd, err := instance.GetFlowIOMetadata(a.flowURI)
			if err != nil {
				log.RootLogger().Warnf("unable to load subflow metadata: %s", err.Error())
				return a.activityMd
			}
			a.activityMd.IOMetadata = flowIOMd

			atomic.StoreUint32(&a.mdUpdated, 1)
		}
	}

	return a.activityMd
}

// Eval implements api.Activity.Eval
func (a *SubFlowActivity) Eval(ctx activity.Context) (done bool, err error) {

	ctx.Logger().Debugf("Starting SubFlow: %s", a.flowURI)

	input := make(map[string]interface{})

	md := a.Metadata()
	if md.IOMetadata != nil {

		for name := range md.Input {
			input[name] = ctx.GetInput(name)
		}
	}

	err = instance.StartSubFlow(ctx, a.flowURI, input)

	return false, nil
}

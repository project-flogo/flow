package subflow

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/support/connection"
	"github.com/project-flogo/core/support/sqltx"
	"github.com/project-flogo/flow/instance"
)

func init() {
	_ = activity.Register(&SubFlowActivity{}, New)

	// FLOGO-19484: flow/instance must not import database/sql, so it carries a transaction
	// registry only through this indirection. Registering here means a binary that links the
	// subflow activity gets real propagation, and one that does not keeps the identity default
	// and allocates nothing.
	instance.SetTxContextPropagator(sqltx.Propagate)
}

type Settings struct {
	FlowURI            string `md:"flowURI,required"`
	DetachedInvocation bool   `md:"detached"`
	ExecTimeout        int64  `md:"execTimeout"`
	Transactional      bool   `md:"transactional"`

	// Connection is deliberately interface{} and NOT connection.Manager.
	//
	// activity.ToMetadata -> metadata.StructToTypedMap -> NewFieldDetails maps a
	// connection.Manager field to data.TypeConnection, which makes BOTH metadata.MapToStruct
	// and metadata.ResolveSettingValue - the latter at flow-DEFINITION-LOAD time - call
	// coerce.ToConnection BEFORE New() can see Transactional. A dangling conn://<uuid>, which
	// the designtime leaves behind when the box is unticked and which a removed app.json
	// connection also produces, would then turn an app that starts today into one that fails
	// to start.
	//
	// interface{} maps to data.TypeAny, whose coercion is a passthrough. We resolve explicitly
	// in New(), and only when Transactional is set.
	TransactionConnection interface{} `md:"transactionConnection"`
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
	act := &SubFlowActivity{flowURI: s.FlowURI, activityMd: activityMd, detachedInvocation: s.DetachedInvocation,
		timeout: s.ExecTimeout, transactional: s.Transactional}

	if s.Transactional {
		if s.DetachedInvocation {
			// Detached is genuinely fire-and-forget: FlowAction.Run signals the handler before
			// the step loop starts, so the subflow outlives this activity and runs on a context
			// this activity never sees. A transaction could not span it.
			return nil, errors.New("SUBFLOW-TX-010: a detached subflow cannot be transactional")
		}
		if s.TransactionConnection == nil {
			return nil, errors.New("SUBFLOW-TX-011: a connection is required when the subflow is transactional")
		}

		mgr, err := coerce.ToConnection(s.TransactionConnection)
		if err != nil {
			return nil, fmt.Errorf("SUBFLOW-TX-012: unable to resolve the transactional subflow's connection: %w", err)
		}
		if mgr == nil {
			return nil, errors.New("SUBFLOW-TX-011: a connection is required when the subflow is transactional")
		}

		// The id must come from the registry, never from a field on the manager. None of the
		// four SQL connectors reliably stores its registry id: mssql declares `name`/`connKey`
		// and assigns neither, postgres never assigns `name`, mysql's `name` is the display
		// name, and oracle has no id field at all. Reading a struct field would make the whole
		// feature a silent no-op.
		connID := connection.GetId(mgr)
		if connID == "" {
			return nil, errors.New("SUBFLOW-TX-013: a transactional subflow requires a SHARED connection (conn://...); an inline connection config cannot be shared with the activities inside the subflow")
		}

		// Best effort only. Managed connectors open their pool in Start(), which the app runs
		// AFTER New(), so GetConnection() is legitimately nil here. Eval repeats this check,
		// where it is mandatory.
		if c := mgr.GetConnection(); c != nil {
			if _, ok := c.(*sql.DB); !ok {
				return nil, fmt.Errorf("SUBFLOW-TX-014: connection '%s' does not expose a *sql.DB", mgr.Type())
			}
		}

		act.connMgr, act.connID = mgr, connID
		ctx.Logger().Debugf("FLOGO-19484: subflow '%s' is transactional on connection id '%s'", s.FlowURI, connID)
	} else if s.TransactionConnection != nil {
		// Reachable precisely because Connection is interface{}: no coercion has happened yet.
		// A stale value left behind by the designtime must not stop the app from starting.
		if _, err := coerce.ToConnection(s.TransactionConnection); err != nil {
			ctx.Logger().Warnf("FLOGO-19484: subflow activity is not transactional and its stale 'connection' setting could not be resolved (%v); ignoring it", err)
		}
	}

	ctx.Logger().Debugf("flowURI: %+v", s.FlowURI)

	return act, nil
}

// SubFlowActivity is an Activity that is used to start a sub-flow, can only be used within the
// context of an flow
// settings: {flowURI}
// input : {sub-flow's input}
// output: {sub-flow's output}
type SubFlowActivity struct {
	activityMd         *activity.Metadata
	flowURI            string
	detachedInvocation bool
	timeout            int64
	mutex              sync.Mutex
	mdUpdated          uint32

	// FLOGO-19484. Set only when the subflow is transactional; connMgr/connID are resolved once
	// in New() and are immutable thereafter, so Eval needs no locking to read them.
	transactional bool
	connMgr       connection.Manager
	connID        string
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

	if a.transactional {
		return a.evalTransactional(ctx, input)
	}

	if a.detachedInvocation {
		ctx.Logger().Infof("Starting SubFlow '%s' in detached mode", a.flowURI)
		err = instance.StartDetachedSubFlow(ctx, a.flowURI, input)
	} else if a.timeout != 0 {
		ctx.Logger().Infof("Starting SubFlow '%s' with timeout '%v'", a.flowURI, a.timeout)
		err = instance.StartSubFlowWithContext(a.timeout, ctx, a.flowURI, input)
	} else {
		err = instance.StartSubFlow(ctx, a.flowURI, input)
	}

	return a.detachedInvocation, err
}

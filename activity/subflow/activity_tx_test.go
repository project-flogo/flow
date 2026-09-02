package subflow

import (
	"context"
	"database/sql"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/project-flogo/core/action"
	"github.com/project-flogo/core/activity"
	"github.com/project-flogo/core/app/resource"
	"github.com/project-flogo/core/support/connection"
	"github.com/project-flogo/core/support/sqltx"
	"github.com/project-flogo/core/support/test"
	"github.com/project-flogo/flow"
	"github.com/project-flogo/flow/definition"
	flowsupport "github.com/project-flogo/flow/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

// fakeConnMgr is a connection.Manager over whatever GetConnection should hand back. A POINTER
// receiver is required: connection.GetId compares managers with ==, and a value receiver over a
// struct holding an uncomparable field would make that panic.
type fakeConnMgr struct {
	typ  string
	conn interface{}
}

var _ connection.Manager = (*fakeConnMgr)(nil)

func (m *fakeConnMgr) Type() string                    { return m.typ }
func (m *fakeConnMgr) GetConnection() interface{}      { return m.conn }
func (m *fakeConnMgr) ReleaseConnection(_ interface{}) {}

var connSeq int32

// registerConn registers mgr as a SHARED connection under a unique id and returns that id.
// connection.RegisterManager rejects duplicates and has no deregister, so the id must be unique
// for the whole test binary.
func registerConn(t *testing.T, mgr connection.Manager) string {
	t.Helper()

	id := "subflow-tx-test-" + strconv.Itoa(int(atomic.AddInt32(&connSeq, 1)))
	require.NoError(t, connection.RegisterManager(id, mgr))

	return id
}

// goCtxActivityContext is test.TestActivityContext with a real GoContext. The stock one always
// returns nil, which is exactly the context evalTransactional's nested guard inspects.
type goCtxActivityContext struct {
	*test.TestActivityContext
	goCtx context.Context
}

func (c *goCtxActivityContext) GoContext() context.Context { return c.goCtx }

// ---------------------------------------------------------------------------
// New() - transactional validation
// ---------------------------------------------------------------------------

func TestNewTransactionalDetachedIsRejected(t *testing.T) {
	settings := map[string]interface{}{
		"flowURI":       "res://flow:flow2",
		"detached":      true,
		"transactional": true,
	}

	act, err := New(test.NewActivityInitContext(settings, nil))

	assert.Nil(t, act)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SUBFLOW-TX-010")
}

func TestNewTransactionalWithoutConnectionIsRejected(t *testing.T) {
	settings := map[string]interface{}{
		"flowURI":       "res://flow:flow2",
		"transactional": true,
	}

	act, err := New(test.NewActivityInitContext(settings, nil))

	assert.Nil(t, act)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SUBFLOW-TX-011")
}

func TestNewTransactionalWithEmptyConnectionIsRejected(t *testing.T) {
	// An empty string coerces to a nil manager rather than an error, so the -011 branch after
	// coerce.ToConnection is the one that has to catch it.
	settings := map[string]interface{}{
		"flowURI":               "res://flow:flow2",
		"transactional":         true,
		"transactionConnection": "",
	}

	act, err := New(test.NewActivityInitContext(settings, nil))

	assert.Nil(t, act)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SUBFLOW-TX-011")
}

func TestNewTransactionalWithUnresolvableConnectionIsRejected(t *testing.T) {
	settings := map[string]interface{}{
		"flowURI":               "res://flow:flow2",
		"transactional":         true,
		"transactionConnection": "conn://no-such-connection-tx-012",
	}

	act, err := New(test.NewActivityInitContext(settings, nil))

	assert.Nil(t, act)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SUBFLOW-TX-012")
}

func TestNewTransactionalWithUnsharedConnectionIsRejected(t *testing.T) {
	db, _ := newFakeDB(t, 0)

	// Handed straight to the activity as a manager instance, exactly like an INLINE connection
	// config: coerce.ToConnection resolves it, but it is not in the shared registry so
	// connection.GetId returns "" and the activities inside the subflow could never find it.
	mgr := &fakeConnMgr{typ: "fake-sql", conn: db}

	settings := map[string]interface{}{
		"flowURI":               "res://flow:flow2",
		"transactional":         true,
		"transactionConnection": mgr,
	}

	act, err := New(test.NewActivityInitContext(settings, nil))

	assert.Nil(t, act)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SUBFLOW-TX-013")
	assert.Equal(t, "", connection.GetId(mgr), "the fixture must not be a shared connection")
}

func TestNewTransactionalWithNonSQLConnectionIsRejected(t *testing.T) {
	mgr := &fakeConnMgr{typ: "kafka-ish", conn: "not a *sql.DB"}
	id := registerConn(t, mgr)

	settings := map[string]interface{}{
		"flowURI":               "res://flow:flow2",
		"transactional":         true,
		"transactionConnection": "conn://" + id,
	}

	act, err := New(test.NewActivityInitContext(settings, nil))

	assert.Nil(t, act)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SUBFLOW-TX-014")
	assert.Contains(t, err.Error(), "kafka-ish")
}

func TestNewTransactionalWithSharedSQLConnectionSucceeds(t *testing.T) {
	db, _ := newFakeDB(t, 0)
	mgr := &fakeConnMgr{typ: "fake-sql", conn: db}
	id := registerConn(t, mgr)

	settings := map[string]interface{}{
		"flowURI":               "res://flow:flow2",
		"transactional":         true,
		"transactionConnection": "conn://" + id,
	}

	act, err := New(test.NewActivityInitContext(settings, nil))
	require.NoError(t, err)

	sfa, ok := act.(*SubFlowActivity)
	require.True(t, ok)
	assert.True(t, sfa.transactional)
	assert.Equal(t, id, sfa.connID, "the connection id must come from the registry")
	assert.Equal(t, connection.Manager(mgr), sfa.connMgr)
	assert.Equal(t, "res://flow:flow2", sfa.flowURI)
}

func TestNewTransactionalToleratesAPoolThatIsNotOpenYet(t *testing.T) {
	// Managed connectors open their pool in Start(), which runs AFTER New(), so GetConnection()
	// legitimately returns nil here. New() must not reject that; Eval re-checks.
	mgr := &fakeConnMgr{typ: "fake-sql", conn: nil}
	id := registerConn(t, mgr)

	settings := map[string]interface{}{
		"flowURI":               "res://flow:flow2",
		"transactional":         true,
		"transactionConnection": "conn://" + id,
	}

	act, err := New(test.NewActivityInitContext(settings, nil))
	require.NoError(t, err)

	sfa := act.(*SubFlowActivity)
	assert.True(t, sfa.transactional)
	assert.Equal(t, id, sfa.connID)
}

// ---------------------------------------------------------------------------
// New() - non-transactional behaviour is unchanged (blocker X4)
// ---------------------------------------------------------------------------

func TestNewNonTransactionalWithoutConnectionIsUnchanged(t *testing.T) {
	settings := map[string]interface{}{"flowURI": "res://flow:flow2"}

	act, err := New(test.NewActivityInitContext(settings, nil))
	require.NoError(t, err)

	sfa := act.(*SubFlowActivity)
	assert.False(t, sfa.transactional)
	assert.Nil(t, sfa.connMgr)
	assert.Equal(t, "", sfa.connID)
	assert.Equal(t, "res://flow:flow2", sfa.flowURI)
}

// TestNewNonTransactionalWithDanglingConnectionMustNotBreakStartup is THE X4 REGRESSION TEST.
//
// WHY IT EXISTS
// -------------
// Settings.Connection is declared `interface{}`, NOT `connection.Manager`. If it were typed
// connection.Manager, activity.ToMetadata -> metadata.StructToTypedMap -> NewFieldDetails would
// map it to data.TypeConnection, and BOTH metadata.MapToStruct here AND
// metadata.ResolveSettingValue - which runs at flow-DEFINITION-LOAD time, before New() is ever
// called - would run coerce.ToConnection on the raw value.
//
// coerce.ToConnection fails hard on a `conn://<uuid>` whose manager is not registered. The
// designtime leaves exactly such a dangling reference behind when the "transactional" box is
// unticked, and so does deleting the connection from app.json. So a typed field would turn an app
// that starts today into one that FAILS TO START - for a setting it is not even using.
//
// interface{} maps to data.TypeAny, whose coercion is a passthrough. New() resolves the value
// explicitly, and only when Transactional is set; otherwise it merely WARNs.
//
// If this test ever fails, the Connection field has been "tidied up" into a typed one. Do not fix
// the test - revert the type.
func TestNewNonTransactionalWithDanglingConnectionMustNotBreakStartup(t *testing.T) {
	settings := map[string]interface{}{
		"flowURI": "res://flow:flow2",
		// transactional deliberately absent, i.e. false
		"transactionConnection": "conn://does-not-exist",
	}

	act, err := New(test.NewActivityInitContext(settings, nil))
	require.NoError(t, err, "a dangling connection on a NON-transactional subflow must not stop the app from starting")
	require.NotNil(t, act)

	sfa := act.(*SubFlowActivity)
	assert.False(t, sfa.transactional)
	assert.Nil(t, sfa.connMgr, "nothing must be resolved on the non-transactional path")
	assert.Equal(t, "", sfa.connID)

	// Deliberately NOT asserting sfa.Metadata() here. Metadata() reaches
	// instance.GetFlowIOMetadata -> flow/support.GetDefinition -> resource.(*Manager).GetResource,
	// which nil-derefs unless some other test in this package happened to install a resource
	// manager first. That made this test pass only by file ordering and panic under -shuffle.
	// The X4 claim is fully carried by New() succeeding above and by
	// TestFlowLoadWithDanglingConnectionMustNotBreakStartup, which drives the real loader path.
}

// TestFlowLoadWithDanglingConnectionMustNotBreakStartup is the same regression, but exercised
// through the code path that actually breaks: the FLOW LOADER.
//
// support.FlowLoader.LoadResource -> materializeFlow -> metadata.ResolveSettingValue (per
// setting, against the activity's declared settings metadata) -> the activity factory. That is
// where a data.TypeConnection-typed setting would blow up, long before anything runs.
func TestFlowLoadWithDanglingConnectionMustNotBreakStartup(t *testing.T) {
	f := action.GetFactory("github.com/project-flogo/flow")
	af := f.(*flow.ActionFactory)
	require.NoError(t, initActionFactory(af)) // sets the expression/mapper factories materializeFlow needs

	const jsonFlowDanglingConn = `{
  "name":"x4-dangling-connection",
  "tasks": [
    {
      "id": "runFlow",
      "activity": {
        "ref": "github.com/project-flogo/flow/activity/subflow",
        "settings": {
          "flowURI": "res://flow:flow2",
          "transactional": false,
          "transactionConnection": "conn://does-not-exist"
        },
        "input": { "in" : "test" }
      }
    }
  ]
}`

	loader := resource.GetLoader(flowsupport.ResTypeFlow)
	require.NotNil(t, loader)

	res, err := loader.LoadResource(&resource.Config{ID: "flow:x4dangling", Data: []byte(jsonFlowDanglingConn)})
	require.NoError(t, err, "loading a flow whose non-transactional subflow carries a stale conn:// must succeed")
	require.NotNil(t, res)

	def, ok := res.Object().(*definition.Definition)
	require.True(t, ok)

	task := def.GetTask("runFlow")
	require.NotNil(t, task)

	sfa, ok := task.ActivityConfig().Activity.(*SubFlowActivity)
	require.True(t, ok, "the subflow activity must have been constructed")
	assert.False(t, sfa.transactional)
	assert.Nil(t, sfa.connMgr)
	assert.Equal(t, "res://flow:flow2", sfa.flowURI)
}

// ---------------------------------------------------------------------------
// evalTransactional guards
// ---------------------------------------------------------------------------

func TestEvalTransactionalRejectsNesting(t *testing.T) {
	db, _ := newFakeDB(t, 0)
	mgr := &fakeConnMgr{typ: "fake-sql", conn: db}

	outer := sqltx.NewHandle("outer-conn", db, nil, context.Background())
	goCtx := sqltx.WithHandle(context.Background(), "outer-conn", outer)

	a := &SubFlowActivity{
		flowURI:       "res://flow:flow2",
		activityMd:    activityMd,
		transactional: true,
		connMgr:       mgr,
		connID:        "inner-conn",
	}

	actCtx := &goCtxActivityContext{TestActivityContext: test.NewActivityContext(activityMd), goCtx: goCtx}

	done, err := a.evalTransactional(actCtx, nil)

	assert.False(t, done)
	require.Error(t, err)

	// The nested guard is returned from Eval, where the retry check does consult Retriable(), so
	// it must be an *activity.Error carrying the code - a misconfiguration is not retriable.
	ae, ok := err.(*activity.Error)
	require.True(t, ok, "expected an *activity.Error, got %T", err)
	assert.Equal(t, "SUBFLOW-TX-002", ae.Code())
	assert.False(t, ae.Retriable())
	assert.Contains(t, ae.Error(), "outer-conn")
}

func TestEvalTransactionalRejectsNonSQLConnectionAtRuntime(t *testing.T) {
	mgr := &fakeConnMgr{typ: "kafka-ish", conn: "not a *sql.DB"}

	a := &SubFlowActivity{
		flowURI:       "res://flow:flow2",
		activityMd:    activityMd,
		transactional: true,
		connMgr:       mgr,
		connID:        "conn-a",
	}

	actCtx := &goCtxActivityContext{TestActivityContext: test.NewActivityContext(activityMd), goCtx: context.Background()}

	done, err := a.evalTransactional(actCtx, nil)

	assert.False(t, done)
	require.Error(t, err)

	ae, ok := err.(*activity.Error)
	require.True(t, ok, "expected an *activity.Error, got %T", err)
	assert.Equal(t, "SUBFLOW-TX-014", ae.Code())
}

func TestEvalTransactionalRejectsAPoolThatWasNeverOpened(t *testing.T) {
	// GetConnection() nil at Eval time is fatal - New() only tolerated it because Start() had not
	// run yet.
	var nilDB *sql.DB
	mgr := &fakeConnMgr{typ: "fake-sql", conn: nilDB}

	a := &SubFlowActivity{
		flowURI:       "res://flow:flow2",
		activityMd:    activityMd,
		transactional: true,
		connMgr:       mgr,
		connID:        "conn-a",
	}

	actCtx := &goCtxActivityContext{TestActivityContext: test.NewActivityContext(activityMd), goCtx: context.Background()}

	done, err := a.evalTransactional(actCtx, nil)

	assert.False(t, done)
	require.Error(t, err)

	ae, ok := err.(*activity.Error)
	require.True(t, ok, "expected an *activity.Error, got %T", err)
	assert.Equal(t, "SUBFLOW-TX-014", ae.Code())
}

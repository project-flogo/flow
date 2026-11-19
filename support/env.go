package support

import (
	"os"
	"sync/atomic"

	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/engine"
)

const (
	UserName   = "FLOGO_APP_USERNAME"
	HostName   = "FLOGO_HOST_NAME"
	AppName    = "FLOGO_APP_NAME"
	AppVersion = "FLOGO_APP_VERSION"

	PropagateSkip             = "FLOGO_TASK_PROPAGATE_SKIP"
	PropagateSkipDefault bool = true

	PrioritizeExprLink             = "FLOGO_TASK_PRIORITIZE_EXPR_LINK"
	PrioritizeExprLinkDefault bool = false

	ConcurrentExecution             = "FLOGO_FLOW_EXECUTE_BRANCHES_CONCURRENTLY"
	ConcurrentExecutionDefault bool = false
)

// The first non-empty value of each variable is cached; the getters run on concurrent flow goroutines.
var username, hostName, appName, appVersion atomic.Pointer[string]

func cachedEnv(cache *atomic.Pointer[string], key string) string {
	if v := cache.Load(); v != nil {
		return *v
	}
	v := os.Getenv(key)
	if len(v) > 0 {
		cache.Store(&v)
	}
	return v
}

func GetUserName() string {
	if v := cachedEnv(&username, UserName); len(v) > 0 {
		return v
	}
	return "flogo"
}

func GetHostId() string {
	if v := cachedEnv(&hostName, HostName); len(v) > 0 {
		return v
	}
	h, _ := os.Hostname()
	return h
}

func GetAppName() string {
	if v := cachedEnv(&appName, AppName); len(v) > 0 {
		return v
	}
	return engine.GetAppName()
}

func GetAppVerison() string {
	if v := cachedEnv(&appVersion, AppVersion); len(v) > 0 {
		return v
	}
	return engine.GetAppVersion()
}

func GetPropagateSkip() bool {
	v, ok := os.LookupEnv(PropagateSkip)
	if !ok {
		return PropagateSkipDefault
	}
	propagateSkip, err := coerce.ToBool(v)
	if err != nil {
		return PropagateSkipDefault
	}
	return propagateSkip
}

func GetPrioritizeExprTask() bool {
	v, ok := os.LookupEnv(PrioritizeExprLink)
	if !ok {
		return PrioritizeExprLinkDefault
	}
	prioritizeExpression, err := coerce.ToBool(v)
	if err != nil {
		return PrioritizeExprLinkDefault
	}
	return prioritizeExpression
}

// GetConcurrentExecution reports whether parallel (concurrent) execution of
// ready tasks/branches is enabled. Default is false (sequential). Controlled by
// the FLOGO_FLOW_EXECUTE_BRANCHES_CONCURRENTLY environment variable.
func GetConcurrentExecution() bool {
	v, ok := os.LookupEnv(ConcurrentExecution)
	if !ok {
		return ConcurrentExecutionDefault
	}
	concurrentExecution, err := coerce.ToBool(v)
	if err != nil {
		return ConcurrentExecutionDefault
	}
	return concurrentExecution
}

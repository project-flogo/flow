package support

import (
	"os"

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

	ConcurrentExecution             = "FLOGO_FLOW_CONCURRENT_TASK_EXECUTION"
	ConcurrentExecutionDefault bool = false
)

var username, hostName, appName, appVersion string

func GetUserName() string {
	if len(username) > 0 {
		return username
	}
	username = os.Getenv(UserName)
	if len(username) > 0 {
		return username
	}
	return "flogo"
}

func GetHostId() string {
	if len(hostName) > 0 {
		return hostName
	}
	hostName = os.Getenv(HostName)
	if len(hostName) > 0 {
		return hostName
	}
	h, _ := os.Hostname()
	return h
}

func GetAppName() string {
	if len(appName) > 0 {
		return appName
	}
	appName = os.Getenv(AppName)
	if len(appName) <= 0 {
		return engine.GetAppName()
	}
	return appName
}

func GetAppVerison() string {
	if len(appVersion) > 0 {
		return appVersion
	}
	appVersion = os.Getenv(AppVersion)
	if len(appVersion) <= 0 {
		return engine.GetAppVersion()
	}
	return appVersion
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
// the FLOGO_FLOW_CONCURRENT_TASK_EXECUTION environment variable.
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

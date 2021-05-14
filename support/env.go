package support

import (
	"github.com/project-flogo/core/engine"
	"os"
)

const (
	UserName = "FLOGO_APP_USERNAME"
	HostName = "FLOGO_HOST_NAME"
	AppName  = "FLOGO_APP_NAME"
)

var username, hostName, appId string

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

func GetAppId() string {
	if len(appId) > 0 {
		return appId
	}
	appId = os.Getenv(AppName)
	if len(appId) <= 0 {
		return engine.GetAppName()
	}
	return appId
}

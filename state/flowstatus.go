package state

import (
	"time"
)

type FlowState struct {
	UserId         string                 `json:"user_id"`
	AppName        string                 `json:"app_name"`
	AppVersion     string                 `json:"app_version"`
	HostId         string                 `json:"host_id"`
	FlowName       string                 `json:"flow_name"`
	FlowInstanceId string                 `json:"flow_instance_id"`
	FlowInputs     map[string]interface{} `json:"flow_inputs"`
	FlowOutputs    map[string]interface{} `json:"flow_outputs"`
	FlowStats      string                 `json:"flow_stats"`
	RerunCount     int                    `json:rerun_count`
	StartTime      time.Time              `json:"start_time"`
	EndTime        time.Time              `json:"end_time"`
}

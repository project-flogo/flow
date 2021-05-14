package state

import (
	"time"
)

type FlowState struct {
	UserId         string `json:"user_id"`
	AppId          string `json:"app_id"`
	HostId         string `json:"host_id"`
	FlowName       string `json:"flow_name"`
	FlowInstanceId string `json:"flow_instance_id"`
	FlowStats      string `json:"flow_stats"`
	//FlowInputs     map[string]interface{} `json:"flow_inputs"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

package state

import (
	"time"

	"github.com/project-flogo/flow/state/change"
)

type Step struct {
	//*Master
	Id           int                   `json:"id"`
	FlowId       string                `json:"flowId"`
	FlowChanges  map[int]*change.Flow  `json:"flowChanges"`
	QueueChanges map[int]*change.Queue `json:"queueChanges,omitempty"`
	StartTime    time.Time             `json:"starttime"`
	EndTime      time.Time             `json:"endtime"`
}

//func (s *Step) SetMasterData(m *Master) {
//	s.Master = m
//}

//func GetMasterData(flowInstanceId string) *Master {
//	return &Master{
//		UserId:     support.GetUserName(),
//		AppId:      support.GetAppId(),
//		HostId:     support.GetHostId(),
//		FlowInstId: flowInstanceId,
//	}
//}

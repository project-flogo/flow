package state

import "github.com/project-flogo/flow/state/change"

type FlowInfo struct {
	Id            string `json:"id"`
	FlowURI       string `json:"flowURI,omitempty"`
	FlowName      string `json:"flowName,omitempty"`
	Status        int    `json:"status,omitempty"`
	HostId        string `json:"hostid,omitempty"`
	FlowStatus    string `json:"flowStatus,omitempty"`
	StartTime     string `json:"starttime,omitempty"`
	EndTime       string `json:"endtime,omitempty"`
	ExecutionTime string `json:"executiontime,omitempty"`
}

func StepsToSnapshot(flowId string, steps []*Step) *Snapshot {

	fs := &Snapshot{Id: flowId, SnapshotBase: &SnapshotBase{}}

	subflows := make(map[int]*Subflow)
	first := -1
	queueDone := false

	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if len(step.FlowChanges) > 0 {
			for _, flowChange := range step.FlowChanges {

				if flowChange.SubflowId > 0 {
					subflow, exists := subflows[flowChange.SubflowId]
					if !exists {
						subflow = &Subflow{
							SnapshotBase: &SnapshotBase{},
							Id:           flowChange.SubflowId,
							TaskId:       flowChange.TaskId,
						}
					}
					UpdateFlow(subflow.SnapshotBase, flowChange)
				} else {
					// update main flow
					UpdateFlow(fs.SnapshotBase, flowChange)
				}
			}
		}
		if len(step.QueueChanges) > 0 && !queueDone {
			for id, queueChg := range step.QueueChanges {
				queueDone = UpdateQueue(fs, id, queueChg, &first)
				if queueDone {
					break
				}
			}
		}

	}

	for id, subflow := range subflows {
		if id != 0 {
			fs.Subflows = append(fs.Subflows, subflow)
		}
	}

	return fs
}

func UpdateQueue(fs *Snapshot, id int, queueChg *change.Queue, first *int) bool {

	if *first == -1 && queueChg.ChgType == change.Delete {
		*first = id
	} else {
		if id == *first {
			return true
		}
	}

	if queueChg.ChgType == change.Add {
		fs.WorkQueue = append(fs.WorkQueue, &WorkItem{
			ID:        id,
			SubflowId: queueChg.SubflowId,
			TaskId:    queueChg.TaskId,
		})
	}
	return false
}

func UpdateFlow(s *SnapshotBase, change *change.Flow) {

	if len(change.Attrs) > 0 {
		if s.Attrs == nil {
			s.Attrs = change.Attrs
		} else {
			for name, value := range change.Attrs {
				s.Attrs[name] = value
			}
		}
	}

	if s.Status == 0 {
		s.Status = change.Status
	}

	if change.NewFlow {
		s.FlowURI = change.FlowURI
	}

	if len(change.Tasks) > 0 {
		for id, taskChg := range change.Tasks {
			UpdateTasks(s, id, taskChg)
		}
	}
	if len(change.Links) > 0 {
		for id, linkChg := range change.Links {
			UpdateLinks(s, id, linkChg)
		}
	}
}

func UpdateTasks(snapshot *SnapshotBase, taskId string, change *change.Task) {

	var td *Task

	if snapshot.Tasks != nil {
		for _, task := range snapshot.Tasks {
			if task.Id == taskId {
				td = task
			}
		}
	}

	if td == nil {
		snapshot.Tasks = append(snapshot.Tasks, &Task{
			Id:     taskId,
			Status: change.Status,
		})
	} else {
		if td.Status == 0 {
			td.Status = change.Status
		}
	}
}

func UpdateLinks(snapshot *SnapshotBase, linkId int, change *change.Link) {
	var ld *Link

	if snapshot.Links != nil {
		for _, link := range snapshot.Links {
			if link.Id == linkId {
				ld = link
			}
		}
	}

	if ld == nil {
		snapshot.Links = append(snapshot.Links, &Link{
			Id:     linkId,
			Status: change.Status,
		})
	} else {
		if ld.Status == 0 {
			ld.Status = change.Status
		}
	}
}

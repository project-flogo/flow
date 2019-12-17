package state

type Snapshot struct {
	*SnapshotBase
	Id        string      `json:"id"`
	WorkQueue []*WorkItem `json:"workQueue,omitempty"`
	Subflows  []*Subflow  `json:"subflows,omitempty"`
}

type Subflow struct {
	*SnapshotBase
	Id     int    `json:"id"`
	TaskId string `json:"taskId"`
}

type SnapshotBase struct {
	FlowURI string                 `json:"flowURI"`
	Status  int                    `json:"status"`
	Attrs   map[string]interface{} `json:"attrs,omitempty"`
	Tasks   []*Task                `json:"tasks,omitempty"`
	Links   []*Link                `json:"links,omitempty"`
}

type WorkItem struct {
	ID        int    `json:"id"`
	SubflowId int    `json:"subflowId"`
	TaskId    string `json:"taskId"`
}

type Task struct {
	Id     string `json:"id"`
	Status int    `json:"status"`
	//Attrs  map[string]interface{} `json:"attrs"`
}

type Link struct {
	Id     int `json:"id"`
	Status int `json:"status"`
}

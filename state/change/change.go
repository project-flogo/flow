package change

// Type denotes the type of change for an object in an instance
type Type int

const (
	// Add denotes an addition
	Add Type = iota
	// Update denotes an update
	Update
	// Delete denotes an deletion
	Delete
)

type Flow struct {
	NewFlow   bool                   `json:"newFlow,omitEmpty"`
	FlowURI   string                 `json:"flowURI,omitEmpty"`
	SubflowId int                    `json:"subflowId,omitEmpty"`
	TaskId    string                 `json:"taskId,omitEmpty"`
	Status    int                    `json:"status,omitEmpty"`
	Attrs     map[string]interface{} `json:"attrs,omitEmpty"`
	Tasks     map[string]*Task       `json:"tasks,omitEmpty"`
	Links     map[int]*Link          `json:"links,omitEmpty"`
}

type Task struct {
	ChgType Type `json:"change"`
	Status  int  `json:"status,omitEmpty"`
}

type Link struct {
	ChgType Type `json:"change"`
	Status  int  `json:"status,omitEmpty"`
}

type Queue struct {
	ChgType   Type   `json:"change"`
	SubflowId int    `json:"subflowId,omitEmpty"`
	TaskId    string `json:"taskId,omitEmpty"`
}

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
	NewFlow    bool                   `json:"newFlow,omitEmpty"`
	FlowURI    string                 `json:"flowURI,omitEmpty"`
	SubflowId  int                    `json:"subflowId,omitEmpty"`
	TaskId     string                 `json:"taskId,omitEmpty"`
	Status     int                    `json:"status,omitEmpty"`
	Attrs      map[string]interface{} `json:"attrs,omitEmpty"`
	Tasks      map[string]*Task       `json:"tasks,omitEmpty"`
	Links      map[int]*Link          `json:"links,omitEmpty"`
	ReturnData map[string]interface{} `json:"returnData,omitEmpty"`
}

type Task struct {
	ChgType Type                   `json:"change"`
	Status  int                    `json:"status,omitEmpty"`
	Input   map[string]interface{} `json:"input,omitEmpty"`
}

type Link struct {
	ChgType Type   `json:"change"`
	Status  int    `json:"status,omitEmpty"`
	From    string `json:"from,omitEmpty"`
	To      string `json:"to,omitEmpty"`
}

type Queue struct {
	ChgType   Type   `json:"change"`
	SubflowId int    `json:"subflowId,omitEmpty"`
	TaskId    string `json:"taskId,omitEmpty"`
}

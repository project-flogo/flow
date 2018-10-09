package model

type FlowStatus int
type TaskStatus int
type LinkStatus int

const (
	// StatusNotStarted indicates that the FlowInstance has not started
	FlowStatusNotStarted FlowStatus = 0

	// StatusActive indicates that the FlowInstance is active
	FlowStatusActive FlowStatus = 100

	// StatusCompleted indicates that the FlowInstance has been completed
	FlowStatusCompleted FlowStatus = 500

	// StatusCancelled indicates that the FlowInstance has been cancelled
	FlowStatusCancelled FlowStatus = 600

	// StatusFailed indicates that the FlowInstance has failed
	FlowStatusFailed FlowStatus = 700

	// TaskStatusNotStarted indicates that the Task has not been started
	TaskStatusNotStarted TaskStatus = 0

	// TaskStatusEntered indicates that the Task has been entered
	TaskStatusEntered TaskStatus = 10

	// TaskStatusReady indicates that the Task is ready
	TaskStatusReady TaskStatus = 20

	// TaskStatusWaiting indicates that the Task is waiting
	TaskStatusWaiting TaskStatus = 30

	// TaskStatusDone indicates that the Task is done
	TaskStatusDone TaskStatus = 40

	// TaskStatusSkipped indicates that the Task was skipped
	TaskStatusSkipped TaskStatus = 50

	// TaskStatusFailed indicates that the Task failed
	TaskStatusFailed TaskStatus = 100

	// LinkStatusFalse indicates that the Link evaluated to false
	LinkStatusFalse LinkStatus = 1

	// LinkStatusTrue indicates that the Link evaluated to true
	LinkStatusTrue LinkStatus = 2

	// LinkStatusSkipped indicates that the Link has been skipped
	LinkStatusSkipped LinkStatus = 3
)

package models

// CheckInWithTask combines a check-in with its associated task information
type CheckInWithTask struct {
	CheckIn *CheckIn
	Task    *Task
}

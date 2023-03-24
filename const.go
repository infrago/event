package event

import "errors"

const (
	NAME = "EVENT"
)

var (
	ErrInvalidConnection = errors.New("Invalid event connection.")
	ErrInvalidDeclare    = errors.New("Invalid event declare.")
	ErrInvalidMsg        = errors.New("Invalid event msg.")
	ErrInvalidWeight     = errors.New("Invalid event connection weight.")
)

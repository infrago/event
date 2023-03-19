package event

import "errors"

const (
	NAME = "EVENT"
)

var (
	errInvalidConnection = errors.New("Invalid event connection.")
	errInvalidNotice     = errors.New("Invalid event notice.")
	errInvalidMsg        = errors.New("Invalid event msg.")
	errInvalidWeight     = errors.New("Invalid event connection weight.")
)

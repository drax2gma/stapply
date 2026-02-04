package actions

import (
	"errors"
	"fmt"
)

// ErrUnknownAction is returned when an action type is not found.
var ErrUnknownAction = errors.New("unknown action type")

// ActionError wraps action execution errors.
type ActionError struct {
	Action string
	Err    error
}

func (e *ActionError) Error() string {
	return fmt.Sprintf("action %s: %v", e.Action, e.Err)
}

func (e *ActionError) Unwrap() error {
	return e.Err
}

// ErrMissingArg returns an error for a missing required argument.
func ErrMissingArg(arg string) error {
	return fmt.Errorf("missing required argument: %s", arg)
}

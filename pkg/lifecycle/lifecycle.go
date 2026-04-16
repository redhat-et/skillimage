package lifecycle

import (
	"errors"
	"fmt"
)

type State string

const (
	Draft      State = "draft"
	Testing    State = "testing"
	Published  State = "published"
	Deprecated State = "deprecated"
	Archived   State = "archived"
)

const StatusAnnotation = "io.skillregistry.status"

var transitions = map[State]State{
	Draft:      Testing,
	Testing:    Published,
	Published:  Deprecated,
	Deprecated: Archived,
}

var errInvalidState = errors.New("invalid lifecycle state")

func ValidTransition(from, to State) bool {
	next, ok := transitions[from]
	return ok && next == to
}

func TagForState(version string, state State) string {
	switch state {
	case Draft:
		return version + "-draft"
	case Testing:
		return version + "-testing"
	case Published:
		return version
	case Deprecated:
		return version
	case Archived:
		return ""
	default:
		return ""
	}
}

func ParseState(s string) (State, error) {
	switch State(s) {
	case Draft, Testing, Published, Deprecated, Archived:
		return State(s), nil
	default:
		return "", fmt.Errorf("%w: %q", errInvalidState, s)
	}
}

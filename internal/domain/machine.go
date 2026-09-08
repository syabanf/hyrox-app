// Package domain holds the studio's business rules as pure Go: no database,
// no HTTP, no framework. Everything here is a function of its arguments, which
// is what makes the money-and-access rules cheap to test exhaustively.
package domain

import "fmt"

// TransitionMap declares the legal moves of a state machine. Statuses are
// never assigned directly by handlers; they go through Transition so an
// illegal move fails loudly instead of corrupting history.
type TransitionMap[S ~string] map[S][]S

// InvalidTransitionError reports a move the machine does not allow.
type InvalidTransitionError[S ~string] struct {
	From S
	To   S
}

func (e InvalidTransitionError[S]) Error() string {
	return fmt.Sprintf("invalid transition from %s to %s", e.From, e.To)
}

// Transition returns `to` when the move is legal, and an error otherwise.
func Transition[S ~string](m TransitionMap[S], from, to S) (S, error) {
	for _, allowed := range m[from] {
		if allowed == to {
			return to, nil
		}
	}
	var zero S
	return zero, InvalidTransitionError[S]{From: from, To: to}
}

// CanTransition reports whether the move is legal, for read-only checks.
func CanTransition[S ~string](m TransitionMap[S], from, to S) bool {
	_, err := Transition(m, from, to)
	return err == nil
}

// contains is the small helper the status-set checks share.
func contains[T comparable](haystack []T, needle T) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

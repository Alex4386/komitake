package main

import (
	"strings"

	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
)

// stateToMode maps the protocol state enum to the user-facing mode name shown
// by `komitake status`.
func stateToMode(state adminv1.State) string {
	switch state {
	case adminv1.State_STATE_RUNNING:
		return "normal"
	case adminv1.State_STATE_PAIRING:
		return "pairing"
	case adminv1.State_STATE_DOWN:
		return "stopped"
	default:
		return strings.ToLower(strings.TrimPrefix(state.String(), "STATE_"))
	}
}

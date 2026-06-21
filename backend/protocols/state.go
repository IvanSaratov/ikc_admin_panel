// Package protocols owns the protocols slice: lifecycle (draft → fixed →
// xml_uploaded → registry_entered → generated → completed, plus cancelled
// reachable from any state), numbering (per-group + per-year sequence with
// optional suffix), and the protocol ↔ training_record participant link.
package protocols

import "errors"

// ProtocolStatus is the typed lifecycle vocabulary for protocols. Mirrors
// the CHECK constraint on protocols.status in 001_initial_schema.sql.
type ProtocolStatus string

const (
	StatusDraft           ProtocolStatus = "draft"
	StatusFixed           ProtocolStatus = "fixed"
	StatusXmlUploaded     ProtocolStatus = "xml_uploaded"
	StatusRegistryEntered ProtocolStatus = "registry_entered"
	StatusGenerated       ProtocolStatus = "generated"
	StatusCompleted       ProtocolStatus = "completed"
	StatusCancelled       ProtocolStatus = "cancelled"
)

// AllStatuses is the canonical vocabulary used for validation and tests.
var AllStatuses = []ProtocolStatus{
	StatusDraft,
	StatusFixed,
	StatusXmlUploaded,
	StatusRegistryEntered,
	StatusGenerated,
	StatusCompleted,
	StatusCancelled,
}

// IsValid reports whether s is one of the known statuses. Used by handler
// URL parameters (e.g. POST /transition?to=...) before we touch the DB.
func (s ProtocolStatus) IsValid() bool {
	for _, candidate := range AllStatuses {
		if s == candidate {
			return true
		}
	}
	return false
}

// ErrInvalidTransition is returned by Transition and CanTransition when the
// caller asks for a status move that the state machine forbids.
var ErrInvalidTransition = errors.New("invalid protocol status transition")

// validTransitions describes the linear lifecycle. `cancelled` is reachable
// from any non-terminal status (the only forbidden source is `completed`
// and `cancelled` itself). The map is the single source of truth; both
// CanTransition and Transition consult it.
var validTransitions = map[ProtocolStatus]map[ProtocolStatus]bool{
	StatusDraft: {
		StatusFixed:     true,
		StatusCancelled: true,
	},
	StatusFixed: {
		StatusXmlUploaded: true,
		StatusCancelled:   true,
	},
	StatusXmlUploaded: {
		StatusRegistryEntered: true,
		StatusCancelled:       true,
	},
	StatusRegistryEntered: {
		StatusGenerated: true,
		StatusCancelled: true,
	},
	StatusGenerated: {
		StatusCompleted: true,
		StatusCancelled: true,
	},
	// StatusCompleted and StatusCancelled are terminal: no outgoing edges.
	StatusCompleted: {},
	StatusCancelled: {},
}

// CanTransition reports whether moving the protocol from `from` to `to` is
// permitted by the state machine. It does NOT consult the database — callers
// still have to read the current row to get the `from` status.
func CanTransition(from, to ProtocolStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

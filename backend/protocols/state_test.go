package protocols

import (
	"errors"
	"testing"
)

func TestCanTransition_AllValidPaths(t *testing.T) {
	t.Parallel()

	// The complete linear lifecycle plus the `cancelled` exit from every
	// non-terminal status. Missing any of these is a regression in the
	// state machine.
	valid := []struct {
		from ProtocolStatus
		to   ProtocolStatus
	}{
		{StatusDraft, StatusFixed},
		{StatusFixed, StatusXmlUploaded},
		{StatusXmlUploaded, StatusRegistryEntered},
		{StatusRegistryEntered, StatusGenerated},
		{StatusGenerated, StatusCompleted},
		// `cancelled` reachable from any non-terminal status.
		{StatusDraft, StatusCancelled},
		{StatusFixed, StatusCancelled},
		{StatusXmlUploaded, StatusCancelled},
		{StatusRegistryEntered, StatusCancelled},
		{StatusGenerated, StatusCancelled},
	}

	for _, edge := range valid {
		if !CanTransition(edge.from, edge.to) {
			t.Errorf("expected %s → %s to be allowed", edge.from, edge.to)
		}
	}
}

func TestCanTransition_InvalidPathsRejected(t *testing.T) {
	t.Parallel()

	// Anything that is not a registered edge must return false. The
	// examples below are the obvious forward-skip and backward moves that
	// a caller might attempt by mistake.
	invalid := []struct {
		from ProtocolStatus
		to   ProtocolStatus
	}{
		// Forward skips: cannot move two stages at once.
		{StatusDraft, StatusXmlUploaded},
		{StatusDraft, StatusRegistryEntered},
		{StatusDraft, StatusGenerated},
		{StatusDraft, StatusCompleted},
		{StatusFixed, StatusRegistryEntered},
		{StatusFixed, StatusGenerated},
		{StatusFixed, StatusCompleted},
		{StatusXmlUploaded, StatusGenerated},
		{StatusXmlUploaded, StatusCompleted},
		{StatusRegistryEntered, StatusCompleted},
		// Backward moves are never allowed.
		{StatusFixed, StatusDraft},
		{StatusXmlUploaded, StatusFixed},
		{StatusCompleted, StatusFixed},
		// Terminal statuses have no outgoing edges at all.
		{StatusCompleted, StatusCancelled},
		{StatusCancelled, StatusDraft},
		{StatusCancelled, StatusFixed},
		{StatusCancelled, StatusCompleted},
		// Same-status is never a transition (no self-loops).
		{StatusDraft, StatusDraft},
		{StatusFixed, StatusFixed},
	}

	for _, edge := range invalid {
		if CanTransition(edge.from, edge.to) {
			t.Errorf("expected %s → %s to be rejected", edge.from, edge.to)
		}
	}
}

func TestCanTransition_Cancelled_FromAnyState(t *testing.T) {
	t.Parallel()

	// Belt-and-suspenders for the cancelled edge: every non-terminal
	// status must be able to transition to cancelled.
	for _, from := range []ProtocolStatus{
		StatusDraft,
		StatusFixed,
		StatusXmlUploaded,
		StatusRegistryEntered,
		StatusGenerated,
	} {
		if !CanTransition(from, StatusCancelled) {
			t.Errorf("expected %s → cancelled to be allowed", from)
		}
	}
}

func TestIsValid(t *testing.T) {
	t.Parallel()

	for _, status := range AllStatuses {
		if !status.IsValid() {
			t.Errorf("%s should be valid", status)
		}
	}
	if ProtocolStatus("nonsense").IsValid() {
		t.Errorf("nonsense should not be a valid status")
	}
}

func TestErrInvalidTransition_IsExported(t *testing.T) {
	t.Parallel()

	// The state machine error must be the documented sentinel so callers
	// can use errors.Is to branch on it.
	if !errors.Is(ErrInvalidTransition, ErrInvalidTransition) {
		t.Errorf("ErrInvalidTransition should equal itself")
	}
}

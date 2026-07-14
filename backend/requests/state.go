package requests

// requestRow status values for the request_rows.status column.
//
// Mirrors the CHECK constraint in the released baseline schema
// (pending | parsed | invalid | applied | skipped). Keep this list and
// the schema constraint in sync — the schema is the source of truth,
// the constants here exist for clarity and for compile-time checks in
// tests/service code.
const (
	RowStatusPending = "pending"
	RowStatusParsed  = "parsed"
	RowStatusInvalid = "invalid"
	RowStatusApplied = "applied"
	RowStatusSkipped = "skipped"
)

// requestTrainingItem status values for request_training_items.status.
//
// Schema CHECK: pending | valid | invalid | duplicate | conflict | applied | skipped.
const (
	ItemStatusPending   = "pending"
	ItemStatusValid     = "valid"
	ItemStatusInvalid   = "invalid"
	ItemStatusDuplicate = "duplicate"
	ItemStatusConflict  = "conflict"
	ItemStatusApplied   = "applied"
	ItemStatusSkipped   = "skipped"
)

// rowTransitions is the allowed state graph for a single request_row.
// A "fresh" row is inserted as pending by ImportRows, parsed into either
// parsed/invalid by the normalizer, then the operator chooses one of
// applied/skipped from the UI. Anything else is rejected so we never
// accidentally re-apply a row or re-parse an applied one.
var rowTransitions = map[string]map[string]bool{
	RowStatusPending: {
		RowStatusParsed:  true,
		RowStatusInvalid: true,
		RowStatusApplied: true, // allow direct apply on edge case (e.g. reimport)
		RowStatusSkipped: true,
	},
	RowStatusParsed: {
		RowStatusInvalid: true,
		RowStatusApplied: true,
		RowStatusSkipped: true,
	},
	RowStatusInvalid: {
		RowStatusParsed:  true, // operator can fix the row in the UI and re-parse
		RowStatusSkipped: true,
		RowStatusApplied: true, // override + apply despite validation
	},
	RowStatusApplied: {}, // terminal
	RowStatusSkipped: {}, // terminal
}

// itemTransitions is the allowed state graph for a single
// request_training_item. The item starts as pending, the normalizer
// moves it to valid/invalid/duplicate/conflict, and the operator
// resolves applied vs skipped.
var itemTransitions = map[string]map[string]bool{
	ItemStatusPending: {
		ItemStatusValid:     true,
		ItemStatusInvalid:   true,
		ItemStatusDuplicate: true,
		ItemStatusConflict:  true,
		ItemStatusSkipped:   true,
		ItemStatusApplied:   true,
	},
	ItemStatusValid: {
		ItemStatusApplied: true,
		ItemStatusSkipped: true,
		ItemStatusInvalid: true, // operator can flag after manual review
	},
	ItemStatusInvalid: {
		ItemStatusSkipped: true,
		ItemStatusValid:   true, // operator can unflag
	},
	ItemStatusDuplicate: {
		ItemStatusApplied: true, // operator override -> creates a second record
		ItemStatusSkipped: true,
	},
	ItemStatusConflict: {
		ItemStatusApplied: true,
		ItemStatusSkipped: true,
		ItemStatusInvalid: true,
	},
	ItemStatusApplied: {}, // terminal
	ItemStatusSkipped: {}, // terminal
}

// CanTransitionRow reports whether request_rows.status can move from
// `from` to `to`. Used by Service.ApplyRow / SkipRow to guard writes
// against concurrent UI clicks or replayed requests.
func CanTransitionRow(from, to string) bool {
	if from == to {
		return true // idempotent no-op
	}
	next, ok := rowTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

// CanTransitionItem reports whether request_training_items.status can
// move from `from` to `to`.
func CanTransitionItem(from, to string) bool {
	if from == to {
		return true
	}
	next, ok := itemTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

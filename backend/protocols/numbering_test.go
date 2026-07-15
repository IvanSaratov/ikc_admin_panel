package protocols

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

func TestParseISODate_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw       string
		wantYear  int
		wantMonth int
	}{
		{"2026-06-15", 2026, 6},
		{"  2026-06-15  ", 2026, 6},
		{"2000-01-01", 2000, 1},
		{"2026-12-31", 2026, 12},
	}

	for _, tc := range cases {
		year, month, err := ParseISODate(tc.raw)
		if err != nil {
			t.Errorf("ParseISODate(%q) err = %v, want nil", tc.raw, err)
			continue
		}
		if year != tc.wantYear || month != tc.wantMonth {
			t.Errorf("ParseISODate(%q) = (%d, %d), want (%d, %d)",
				tc.raw, year, month, tc.wantYear, tc.wantMonth)
		}
	}
}

func TestParseISODate_Invalid(t *testing.T) {
	t.Parallel()

	bad := []string{
		"",
		"2026-6-15",
		"2026/06/15",
		"2026-06-15T10:00:00Z",
		"abcd-06-15",
		"2026-13-15",
		"2026-00-15",
		"2026-06-32",
		"2026-06-00",
		"1999-06-15",
	}

	for _, raw := range bad {
		if _, _, err := ParseISODate(raw); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("ParseISODate(%q) err = %v, want ErrInvalidDate", raw, err)
		}
	}
}

func TestNormalizeSuffix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"   ", "", false},
		{"1", "1", false},
		{"  3  ", "3", false},
		{"12345", "12345", false},
		{"abc", "", true},
		{"1.5", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeSuffix(tc.raw)
		if tc.wantErr {
			if !errors.Is(err, ErrInvalidSuffix) {
				t.Errorf("NormalizeSuffix(%q) err = %v, want ErrInvalidSuffix", tc.raw, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeSuffix(%q) err = %v, want nil", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizeSuffix(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestFormatProtocolNumber(t *testing.T) {
	t.Parallel()

	cases := []struct {
		year, month int
		seq         int64
		suffix      string
		want        string
	}{
		{2026, 6, 1, "", "2026-06/001"},
		{2026, 6, 1, "1", "2026-06/001/1"},
		{2026, 6, 12, "2", "2026-06/012/2"},
		{2026, 12, 100, "3", "2026-12/100/3"},
		{2026, 1, 999, "", "2026-01/999"},
	}

	for _, tc := range cases {
		got := FormatProtocolNumber(tc.year, tc.month, tc.seq, tc.suffix)
		if got != tc.want {
			t.Errorf("FormatProtocolNumber(%d, %d, %d, %q) = %q, want %q",
				tc.year, tc.month, tc.seq, tc.suffix, got, tc.want)
		}
	}
}

func TestNextSequence_FirstInYear_Returns1(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	next, err := nextSequenceLocked(ctx, env.queries, 999, 2026, "")
	if err != nil {
		t.Fatalf("nextSequenceLocked: %v", err)
	}
	if next != 1 {
		t.Errorf("next = %d, want 1", next)
	}
}

func TestNextSequence_SecondInYear_Returns2(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol: %v", err)
	}
	if _, err := env.svc.Fix(ctx, p.ID, validFixInput()); err != nil {
		t.Fatalf("first Fix: %v", err)
	}

	next, err := nextSequenceLocked(ctx, env.queries, groupID, 2026, "")
	if err != nil {
		t.Fatalf("nextSequenceLocked: %v", err)
	}
	if next != 2 {
		t.Errorf("next = %d, want 2", next)
	}
}

func TestFix_WithSuffix_SharesSequenceNumberAcrossSuffixes(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	// First protocol: no suffix. Fix assigns sequence 1.
	p1, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol p1: %v", err)
	}
	fixed1, err := env.svc.Fix(ctx, p1.ID, validFixInput())
	if err != nil {
		t.Fatalf("Fix p1: %v", err)
	}
	if fixed1.AnnualSequenceNumber.Int64 != 1 {
		t.Fatalf("p1 sequence = %d, want 1", fixed1.AnnualSequenceNumber.Int64)
	}
	if fixed1.ProtocolSuffix.Valid {
		t.Fatalf("p1 suffix should be NULL, got %q", fixed1.ProtocolSuffix.String)
	}
	if fixed1.ProtocolNumber.String != "2026-06/001" {
		t.Errorf("p1 protocol_number = %q, want 2026-06/001", fixed1.ProtocolNumber.String)
	}

	// Second protocol: suffix "1". The unique index uses COALESCE(suffix, '')
	// so this can share (group, year, seq) with p1 only because p1 has no
	// suffix. Fix must still produce seq=1 for this row.
	p2, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol p2: %v", err)
	}
	fixed2, err := env.svc.Fix(ctx, p2.ID, FixInput{
		TrainingStartDate: "2026-06-05",
		TrainingEndDate:   "2026-06-25",
		ProtocolDate:      "2026-06-30",
		ProtocolSuffix:    "1",
	})
	if err != nil {
		t.Fatalf("Fix p2: %v", err)
	}
	if fixed2.AnnualSequenceNumber.Int64 != 1 {
		t.Errorf("p2 sequence = %d, want 1 (shared with p1)", fixed2.AnnualSequenceNumber.Int64)
	}
	if !fixed2.ProtocolSuffix.Valid || fixed2.ProtocolSuffix.String != "1" {
		t.Errorf("p2 suffix = %v, want \"1\"", fixed2.ProtocolSuffix)
	}
	if fixed2.ProtocolNumber.String != "2026-06/001/1" {
		t.Errorf("p2 protocol_number = %q, want 2026-06/001/1", fixed2.ProtocolNumber.String)
	}

	// Third protocol: suffix "2" also shares seq=1 with both above.
	p3, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol p3: %v", err)
	}
	fixed3, err := env.svc.Fix(ctx, p3.ID, FixInput{
		TrainingStartDate: "2026-06-10",
		TrainingEndDate:   "2026-06-20",
		ProtocolDate:      "2026-06-30",
		ProtocolSuffix:    "2",
	})
	if err != nil {
		t.Fatalf("Fix p3: %v", err)
	}
	if fixed3.AnnualSequenceNumber.Int64 != 1 {
		t.Errorf("p3 sequence = %d, want 1 (shared with p1, p2)", fixed3.AnnualSequenceNumber.Int64)
	}
	if fixed3.ProtocolNumber.String != "2026-06/001/2" {
		t.Errorf("p3 protocol_number = %q, want 2026-06/001/2", fixed3.ProtocolNumber.String)
	}

	// A non-suffixed second protocol in the same (group, year) MUST get
	// seq=2 because COALESCE('', '1') != COALESCE(NULL, '').
	p4, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol p4: %v", err)
	}
	fixed4, err := env.svc.Fix(ctx, p4.ID, FixInput{
		TrainingStartDate: "2026-06-12",
		TrainingEndDate:   "2026-06-18",
		ProtocolDate:      "2026-06-30",
	})
	if err != nil {
		t.Fatalf("Fix p4: %v", err)
	}
	if fixed4.AnnualSequenceNumber.Int64 != 2 {
		t.Errorf("p4 sequence = %d, want 2 (next non-suffixed slot)", fixed4.AnnualSequenceNumber.Int64)
	}
	if fixed4.ProtocolNumber.String != "2026-06/002" {
		t.Errorf("p4 protocol_number = %q, want 2026-06/002", fixed4.ProtocolNumber.String)
	}
}

func TestFix_DifferentYears_IndependentSequences(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()
	groupID := env.mustGroup(t)

	p1, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol p1: %v", err)
	}
	fixed1, err := env.svc.Fix(ctx, p1.ID, FixInput{
		TrainingStartDate: "2025-12-01",
		TrainingEndDate:   "2025-12-30",
		ProtocolDate:      "2025-12-30",
	})
	if err != nil {
		t.Fatalf("Fix p1: %v", err)
	}
	if fixed1.SequenceYear.Int64 != 2025 {
		t.Fatalf("p1 year = %d, want 2025", fixed1.SequenceYear.Int64)
	}
	if fixed1.AnnualSequenceNumber.Int64 != 1 {
		t.Fatalf("p1 seq = %d, want 1", fixed1.AnnualSequenceNumber.Int64)
	}

	p2, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: groupID})
	if err != nil {
		t.Fatalf("CreateProtocol p2: %v", err)
	}
	fixed2, err := env.svc.Fix(ctx, p2.ID, validFixInput())
	if err != nil {
		t.Fatalf("Fix p2: %v", err)
	}
	if fixed2.SequenceYear.Int64 != 2026 {
		t.Fatalf("p2 year = %d, want 2026", fixed2.SequenceYear.Int64)
	}
	if fixed2.AnnualSequenceNumber.Int64 != 1 {
		t.Errorf("p2 seq = %d, want 1 (independent of 2025 sequence)", fixed2.AnnualSequenceNumber.Int64)
	}
}

func TestFix_DifferentGroups_IndependentSequences(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t)
	ctx := context.Background()

	g1 := env.mustGroupNamed(t, "G1")
	g2 := env.mustGroupNamed(t, "G2")

	if g1 == g2 {
		t.Fatalf("g1 == g2 = %d, mustGroupNamed broken", g1)
	}

	// Each group runs in a different month so the protocol_number strings
	// don't collide (protocol_number is globally unique; the sequence
	// number itself is per-group and should start at 1 independently).
	p1, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: g1})
	if err != nil {
		t.Fatalf("CreateProtocol p1: %v", err)
	}
	fixed1, err := env.svc.Fix(ctx, p1.ID, FixInput{
		TrainingStartDate: "2026-06-01",
		TrainingEndDate:   "2026-06-30",
		ProtocolDate:      "2026-06-30",
	})
	if err != nil {
		t.Fatalf("Fix p1: %v", err)
	}
	if fixed1.AnnualSequenceNumber.Int64 != 1 {
		t.Fatalf("p1 (g1) seq = %d, want 1", fixed1.AnnualSequenceNumber.Int64)
	}

	p2, err := env.svc.CreateProtocol(ctx, CreateProtocolInput{ProgramGroupID: g2})
	if err != nil {
		t.Fatalf("CreateProtocol p2: %v", err)
	}
	fixed2, err := env.svc.Fix(ctx, p2.ID, FixInput{
		TrainingStartDate: "2026-07-01",
		TrainingEndDate:   "2026-07-30",
		ProtocolDate:      "2026-07-30",
	})
	if err != nil {
		t.Fatalf("Fix p2: %v", err)
	}
	if fixed2.AnnualSequenceNumber.Int64 != 1 {
		t.Errorf("p2 (g2) seq = %d, want 1 (independent per group)", fixed2.AnnualSequenceNumber.Int64)
	}
}

// --- helpers ---

// testEnv bundles the per-test environment so tests can use the service
// alongside the raw DB and the queries facade. Keeps helpers in one place
// so tests don't each open their own DB.
type testEnv struct {
	svc     *Service
	db      *sql.DB
	queries *storagedb.Queries
	// trainingRecordSeq is incremented for each mustTrainingRecord call so
	// repeated calls in the same test produce UNIQUE-compatible rows.
	trainingRecordSeq int
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx := context.Background()
	database, err := storage.Open(ctx, filepath.Join(t.TempDir(), "ikc-test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := storage.Migrate(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	queries := storagedb.New(database)
	auditSvc := audit.NewService(queries)
	return &testEnv{
		svc:     NewService(queries, database, auditSvc),
		db:      database,
		queries: queries,
	}
}

func newService(t *testing.T) *Service { return newTestEnv(t).svc }

func newServiceWithQueries(t *testing.T) (*Service, *storagedb.Queries) {
	env := newTestEnv(t)
	return env.svc, env.queries
}

// mustGroup inserts an active program group with a name derived from the
// test name and returns its id.
func (e *testEnv) mustGroup(t *testing.T) int64 {
	return e.mustGroupNamed(t, "G")
}

// mustGroupNamed lets a single test create multiple groups with distinct
// codes (the code column has a UNIQUE index).
func (e *testEnv) mustGroupNamed(t *testing.T, suffix string) int64 {
	t.Helper()
	e.trainingRecordSeq++
	ctx := context.Background()
	now := "2026-06-21T00:00:00Z"
	res, err := e.db.ExecContext(ctx,
		`INSERT INTO program_groups (code, name, status, created_at, updated_at) VALUES (?, ?, 'active', ?, ?)`,
		fmt.Sprintf("%s-%s-%d", suffix, t.Name(), e.trainingRecordSeq),
		fmt.Sprintf("Group %s %s %d", suffix, t.Name(), e.trainingRecordSeq),
		now, now)
	if err != nil {
		t.Fatalf("insert group: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// validFixInput returns a FixInput with dates that yield sequence_year=2026
// and protocol_month=06, so the protocol_number round-trips through the
// canonical format the tests assert against.
func validFixInput() FixInput {
	return FixInput{
		TrainingStartDate: "2026-06-01",
		TrainingEndDate:   "2026-06-30",
		ProtocolDate:      "2026-06-30",
	}
}

package storage

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"testing"
	"testing/fstest"
	"time"
)

func migrationFS(includeSecond bool) fstest.MapFS {
	migrations := fstest.MapFS{
		"001_items.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
PRAGMA application_id = 0x494B4341;
CREATE TABLE items (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

-- +goose Down
DROP TABLE items;
`)},
	}
	if includeSecond {
		migrations["002_item_status.sql"] = &fstest.MapFile{Data: []byte(`-- +goose Up
ALTER TABLE items ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

-- +goose Down
ALTER TABLE items DROP COLUMN status;
`)}
	}
	return migrations
}

func TestMigratorStatusAndUpAreIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "migrator.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migrator, err := NewMigrator(db, migrationFS(true))
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}

	wantPending := []MigrationInfo{
		{Version: 1, Name: "001_items.sql"},
		{Version: 2, Name: "002_item_status.sql"},
	}
	catalog := migrator.Catalog()
	assertMigrationStatus(t, catalog, 0, 2, wantPending)

	status, err := migrator.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	assertMigrationStatus(t, status, 0, 2, wantPending)

	first, err := migrator.Up(ctx)
	if err != nil {
		t.Fatalf("first Up() error = %v", err)
	}
	if first.From != 0 || first.To != 2 || len(first.Applied) != 2 {
		t.Fatalf("first Up() = %+v, want From 0, To 2, 2 applied migrations", first)
	}

	second, err := migrator.Up(ctx)
	if err != nil {
		t.Fatalf("second Up() error = %v", err)
	}
	if second.From != 2 || second.To != 2 || len(second.Applied) != 0 {
		t.Fatalf("second Up() = %+v, want From 2, To 2, no applied migrations", second)
	}
}

func TestMigratorRejectsSchemaNewerThanBinary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "migrator.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	current, err := NewMigrator(db, migrationFS(true))
	if err != nil {
		t.Fatalf("NewMigrator(current) error = %v", err)
	}
	if _, err := current.Up(ctx); err != nil {
		t.Fatalf("current Up() error = %v", err)
	}

	old, err := NewMigrator(db, migrationFS(false))
	if err != nil {
		t.Fatalf("NewMigrator(old) error = %v", err)
	}
	_, err = old.Up(ctx)
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("old Up() error = %v, want ErrSchemaTooNew", err)
	}
}

func TestMigratorPreservesPartialFailureWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "migrator.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migrator, err := NewMigrator(db, slowMigrationFS())
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	if _, err := migrator.Status(ctx); err != nil {
		t.Fatalf("initial Status() error = %v", err)
	}
	observer, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open(observer) error = %v", err)
	}
	t.Cleanup(func() { _ = observer.Close() })

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type outcome struct {
		result MigrationResult
		err    error
	}
	outcomes := make(chan outcome, 1)
	go func() {
		result, err := migrator.Up(cancelCtx)
		outcomes <- outcome{result: result, err: err}
	}()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	for {
		var version int64
		err := observer.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(version_id), 0)
			FROM goose_db_version
			WHERE is_applied = 1
		`).Scan(&version)
		if err != nil {
			t.Fatalf("observe migration version error = %v", err)
		}
		if version == 1 {
			cancel()
			break
		}
		select {
		case outcome := <-outcomes:
			t.Fatalf("Up() completed before cancellation: result = %+v, error = %v", outcome.result, outcome.err)
		case <-deadline.C:
			cancel()
			t.Fatal("timed out waiting for migration 1 to commit")
		case <-poll.C:
		}
	}
	completed := <-outcomes
	result, err := completed.result, completed.err

	var failure *MigrationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Up() error = %v, want *MigrationFailure", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("Up() error = %v, want canceled context as root cause", err)
	}
	assertMigrationResult(t, result, 0, 1, []int64{1})
	if failure.From != 0 || failure.To != 1 || failure.Target != 2 {
		t.Errorf("failure versions = From %d, To %d, Target %d; want 0, 1, 2", failure.From, failure.To, failure.Target)
	}
	if len(failure.Applied) != 1 || failure.Applied[0].Version != 1 {
		t.Errorf("failure Applied = %+v, want migration 1", failure.Applied)
	}
	if failure.Failed.Version != 2 || failure.Failed.Name != "002_slow.sql" {
		t.Errorf("failure Failed = %+v, want migration 2 (002_slow.sql)", failure.Failed)
	}

	status, err := migrator.Status(context.Background())
	if err != nil {
		t.Fatalf("later Status() error = %v", err)
	}
	assertMigrationStatus(t, status, 1, 2, []MigrationInfo{{Version: 2, Name: "002_slow.sql"}})
}

func TestMigratorReportsOrdinaryPartialFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "migrator.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migrator, err := NewMigrator(db, ordinaryFailureMigrationFS())
	if err != nil {
		t.Fatalf("NewMigrator() error = %v", err)
	}
	result, err := migrator.Up(ctx)

	var failure *MigrationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Up() error = %v, want *MigrationFailure", err)
	}
	assertMigrationResult(t, result, 0, 1, []int64{1})
	if failure.From != 0 || failure.To != 1 || failure.Target != 2 {
		t.Errorf("failure versions = From %d, To %d, Target %d; want 0, 1, 2", failure.From, failure.To, failure.Target)
	}
	if len(failure.Applied) != 1 || failure.Applied[0].Version != 1 || failure.Failed.Version != 2 {
		t.Errorf("failure migrations = Applied %+v, Failed %+v; want applied 1, failed 2", failure.Applied, failure.Failed)
	}
	if failure.Err == nil || !errors.Is(err, failure.Err) {
		t.Errorf("failure root error = %v, want it discoverable through Unwrap", failure.Err)
	}
}

func TestMigratorSerializesConcurrentUpAcrossInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "migrator.db")
	db1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open(first) error = %v", err)
	}
	t.Cleanup(func() { _ = db1.Close() })
	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open(second) error = %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })

	migrator1, err := NewMigrator(db1, concurrentMigrationFS())
	if err != nil {
		t.Fatalf("NewMigrator(first) error = %v", err)
	}
	migrator2, err := NewMigrator(db2, concurrentMigrationFS())
	if err != nil {
		t.Fatalf("NewMigrator(second) error = %v", err)
	}
	for name, migrator := range map[string]*Migrator{"first": migrator1, "second": migrator2} {
		if _, err := migrator.Status(ctx); err != nil {
			t.Fatalf("initial Status(%s) error = %v", name, err)
		}
	}

	type outcome struct {
		result MigrationResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	for _, migrator := range []*Migrator{migrator1, migrator2} {
		go func(migrator *Migrator) {
			<-start
			result, err := migrator.Up(ctx)
			outcomes <- outcome{result: result, err: err}
		}(migrator)
	}
	close(start)

	results := make([]MigrationResult, 0, 2)
	for range 2 {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Fatalf("concurrent Up() error = %v", outcome.err)
		}
		results = append(results, outcome.result)
	}
	sort.Slice(results, func(i, j int) bool { return len(results[i].Applied) > len(results[j].Applied) })
	assertMigrationResult(t, results[0], 0, 2, []int64{1, 2})
	assertMigrationResult(t, results[1], 2, 2, nil)
}

func slowMigrationFS() fstest.MapFS {
	return twoMigrationFS(`CREATE TABLE slow AS
WITH RECURSIVE cnt(x) AS (
    VALUES(0)
    UNION ALL
    SELECT x+1 FROM cnt WHERE x < 1000000000
)
SELECT sum(x) AS n FROM cnt;`, "002_slow.sql")
}

func ordinaryFailureMigrationFS() fstest.MapFS {
	return twoMigrationFS("CREATE TABLE items (id INTEGER PRIMARY KEY);", "002_duplicate.sql")
}

func concurrentMigrationFS() fstest.MapFS {
	return twoMigrationFS(`CREATE TABLE slow AS
WITH RECURSIVE cnt(x) AS (
    VALUES(0)
    UNION ALL
    SELECT x+1 FROM cnt WHERE x < 1000000
)
SELECT sum(x) AS n FROM cnt;`, "002_slow.sql")
}

func twoMigrationFS(secondUp, secondName string) fstest.MapFS {
	return fstest.MapFS{
		"001_items.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
-- +goose Down
DROP TABLE items;
`)},
		secondName: &fstest.MapFile{Data: []byte("-- +goose Up\n" + secondUp + "\n-- +goose Down\n")},
	}
}

func assertMigrationResult(t *testing.T, got MigrationResult, from, to int64, applied []int64) {
	t.Helper()
	if got.From != from || got.To != to {
		t.Errorf("result versions = (%d, %d), want (%d, %d)", got.From, got.To, from, to)
	}
	if len(got.Applied) != len(applied) {
		t.Fatalf("result Applied = %+v, want versions %v", got.Applied, applied)
	}
	for i, version := range applied {
		if got.Applied[i].Version != version {
			t.Errorf("result Applied[%d] = %+v, want version %d", i, got.Applied[i], version)
		}
	}
}

func assertMigrationStatus(t *testing.T, got MigrationStatus, current, target int64, pending []MigrationInfo) {
	t.Helper()
	if got.Current != current || got.Target != target {
		t.Errorf("status versions = (%d, %d), want (%d, %d)", got.Current, got.Target, current, target)
	}
	if len(got.Pending) != len(pending) {
		t.Fatalf("status pending = %+v, want %+v", got.Pending, pending)
	}
	for i := range pending {
		if got.Pending[i] != pending[i] {
			t.Errorf("status pending[%d] = %+v, want %+v", i, got.Pending[i], pending[i])
		}
	}
}

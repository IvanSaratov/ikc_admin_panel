package audit_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/audit"
	storagedb "github.com/IvanSaratov/ikc_admin_panel/backend/storage/db"
)

// TestList_GET_RendersEvents exercises the full happy path: a fresh
// DB is opened, seeded with three action_log rows via the audit
// service, then /audit is fetched as an authenticated operator. The
// response must include the row data and the "Page 1 of 1 (3 entries)"
// footer.
func TestList_GET_RendersEvents(t *testing.T) {
	t.Parallel()

	dbPath := pathInTempDir(t, "events.db")
	if err := openAndMigrate(dbPath); err != nil {
		t.Fatalf("open+migrate: %v", err)
	}
	db := openDB(t, dbPath)
	queries := storagedb.New(db)
	svc := audit.NewService(queries)
	seedAdminUser(t, db)

	// Seed via the production path (audit.Service.Record) so the JSON
	// serialization matches what the UI will render in production.
	ctx := context.Background()
	for _, r := range []struct {
		actor, action, entity string
	}{
		{"alice", "create", "employer"},
		{"alice", "create", "employer"},
		{"bob", "update", "program"},
	} {
		if err := svc.Record(ctx, audit.RecordInput{
			Actor:      r.actor,
			Action:     r.action,
			EntityType: r.entity,
			EntityID:   sql.NullInt64{Int64: 1, Valid: true},
			Details:    map[string]any{"raw": "seed"},
		}); err != nil {
			t.Fatalf("seed %+v: %v", r, err)
		}
	}

	router := routerWithDB(t, db)
	cookies := loginRoundTrip(t, router)

	rec := authedGet(t, router, "/audit", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Audit log", "alice", "bob", "employer", "program"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
	// 3 rows seeded via audit.Service.Record + 1 auto row written
	// by the loginRoundTrip itself (actor=admin, action=login.success,
	// entity_type=session) → footer must say "4 entries". The login
	// row's existence is a fixed cost of going through the real /login
	// path so we count it rather than try to bypass it.
	if !strings.Contains(body, "4 entries") {
		t.Errorf("body missing '4 entries' counter: %s", body)
	}
	// Full page render includes the shell brand.
	if !strings.Contains(body, "Mintrud Admin") {
		t.Errorf("body missing shell brand: %s", body)
	}
}

func TestList_Filters_ByActor(t *testing.T) {
	t.Parallel()

	dbPath := pathInTempDir(t, "actor.db")
	if err := openAndMigrate(dbPath); err != nil {
		t.Fatalf("open+migrate: %v", err)
	}
	db := openDB(t, dbPath)
	seedAdminUser(t, db)
	seedActionLog(t, db)

	router := routerWithDB(t, db)
	cookies := loginRoundTrip(t, router)

	rec := authedGet(t, router, "/audit?actor=alice", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, ">bob<") {
		t.Errorf("actor=alice response still shows bob row: %s", body)
	}
	if strings.Contains(body, ">system<") {
		t.Errorf("actor=alice response still shows system row: %s", body)
	}
	// 3 alice rows were seeded (import, deactivate, create).
	if !strings.Contains(body, "3 entries") {
		t.Errorf("actor=alice counter expected '3 entries': %s", body)
	}
}

func TestList_Filters_ByEntityType(t *testing.T) {
	t.Parallel()

	dbPath := pathInTempDir(t, "entity.db")
	if err := openAndMigrate(dbPath); err != nil {
		t.Fatalf("open+migrate: %v", err)
	}
	db := openDB(t, dbPath)
	seedAdminUser(t, db)
	seedActionLog(t, db)

	router := routerWithDB(t, db)
	cookies := loginRoundTrip(t, router)

	rec := authedGet(t, router, "/audit?entity_type=program", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// We look for entity_types rendered inside a table cell — the
	// surrounding HTML keeps the false-positive rate low (navbar URLs
	// like href="/employers" are NOT inside <td>). If the entity_type
	// filter were broken, the table would render <td>program_group #7
	// </td>, <td>employer #42</td>, or <td><span
	// class="text-secondary">session</span></td>.
	if strings.Contains(body, "<td>program_group") {
		t.Errorf("entity_type=program response still shows program_group row: %s", body)
	}
	if strings.Contains(body, "<td>employer") {
		t.Errorf("entity_type=program response still shows employer row: %s", body)
	}
	if strings.Contains(body, "<td><span class=\"text-secondary\">session") {
		t.Errorf("entity_type=program response still shows session row: %s", body)
	}
	if !strings.Contains(body, "2 entries") {
		t.Errorf("entity_type=program counter expected '2 entries': %s", body)
	}
}

func TestList_Filters_ByDateRange(t *testing.T) {
	t.Parallel()

	dbPath := pathInTempDir(t, "dates.db")
	if err := openAndMigrate(dbPath); err != nil {
		t.Fatalf("open+migrate: %v", err)
	}
	db := openDB(t, dbPath)
	seedAdminUser(t, db)
	seedActionLog(t, db)

	router := routerWithDB(t, db)
	cookies := loginRoundTrip(t, router)

	// created_from=2026-06-01 → 3 seeded rows from June onwards
	// (system/login 06-01, alice/deactivate 06-15, alice/create 06-20)
	// PLUS the loginRoundTrip's own login.success row which is created
	// at the real "now" timestamp (today is 2026-06-21, so it lands
	// on/after 2026-06-01) → 4 entries total.
	rec := authedGet(t, router, "/audit?created_from=2026-06-01", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "4 entries") {
		t.Errorf("created_from counter expected '4 entries': %s", body)
	}
	// The May rows must not appear — their created_at text starts with
	// "2026-05" which is rendered into the table cell.
	if strings.Contains(body, "2026-05-01T10:00:00Z") {
		t.Errorf("created_from=2026-06-01 still shows 2026-05-01 row: %s", body)
	}
	if strings.Contains(body, "2026-05-15T10:00:00Z") {
		t.Errorf("created_from=2026-06-01 still shows 2026-05-15 row: %s", body)
	}

	// Tight range: only 2026-06-15 → 2026-06-19 (alice/deactivate).
	// The login.success row is written at "now" (2026-06-21) which is
	// outside the upper bound 2026-06-19, so it does not pollute the
	// count here → still 1 entry.
	rec = authedGet(t, router, "/audit?created_from=2026-06-15&created_to=2026-06-19", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "1 entries") {
		t.Errorf("tight range counter expected '1 entries': %s", body)
	}
	if strings.Contains(body, "2026-06-20T10:00:00Z") {
		t.Errorf("tight range still shows 06-20 row: %s", body)
	}
}

func TestList_Pagination_NextPrevLinks(t *testing.T) {
	t.Parallel()

	dbPath := pathInTempDir(t, "pagination.db")
	if err := openAndMigrate(dbPath); err != nil {
		t.Fatalf("open+migrate: %v", err)
	}
	db := openDB(t, dbPath)
	seedAdminUser(t, db)
	seedBulkActionLog(t, db, 120)

	router := routerWithDB(t, db)
	cookies := loginRoundTrip(t, router)

	// Page 1 — should NOT render "Previous", must render "Next".
	rec := authedGet(t, router, "/audit", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("page 1 status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, ">Previous<") {
		t.Errorf("page 1 unexpectedly shows Previous link: %s", body)
	}
	if !strings.Contains(body, ">Next<") {
		t.Errorf("page 1 missing Next link: %s", body)
	}
	if !strings.Contains(body, "Page 1 of 3") {
		t.Errorf("page 1 footer expected 'Page 1 of 3': %s", body)
	}

	// Page 2 — both links.
	rec = authedGet(t, router, "/audit?page=2", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d, want 200", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, ">Previous<") {
		t.Errorf("page 2 missing Previous link: %s", body)
	}
	if !strings.Contains(body, ">Next<") {
		t.Errorf("page 2 missing Next link: %s", body)
	}
	if !strings.Contains(body, "Page 2 of 3") {
		t.Errorf("page 2 footer expected 'Page 2 of 3': %s", body)
	}

	// Page 3 (last) — should NOT render "Next", must render "Previous".
	rec = authedGet(t, router, "/audit?page=3", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("page 3 status = %d, want 200", rec.Code)
	}
	body = rec.Body.String()
	if strings.Contains(body, ">Next<") {
		t.Errorf("page 3 unexpectedly shows Next link: %s", body)
	}
	if !strings.Contains(body, ">Previous<") {
		t.Errorf("page 3 missing Previous link: %s", body)
	}
	if !strings.Contains(body, "Page 3 of 3") {
		t.Errorf("page 3 footer expected 'Page 3 of 3': %s", body)
	}
}

func TestList_EmptyState(t *testing.T) {
	t.Parallel()

	dbPath := pathInTempDir(t, "empty.db")
	if err := openAndMigrate(dbPath); err != nil {
		t.Fatalf("open+migrate: %v", err)
	}
	db := openDB(t, dbPath)
	seedAdminUser(t, db)

	router := routerWithDB(t, db)
	cookies := loginRoundTrip(t, router)

	// The test sets a filter that no seeded row can match: actor=ghost.
	// Going through loginRoundTrip itself writes a login.success row
	// with actor=admin, which is excluded by this filter. So the audit
	// table really IS empty for this query → 0 entries.
	rec := authedGet(t, router, "/audit?actor=ghost", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Audit log") {
		t.Errorf("body missing Audit log heading: %s", body)
	}
	if !strings.Contains(body, "No audit entries match") {
		t.Errorf("body missing empty-state copy: %s", body)
	}
	if !strings.Contains(body, "0 entries") {
		t.Errorf("body missing '0 entries' counter: %s", body)
	}
	// Empty filter → no Previous/Next links.
	if strings.Contains(body, ">Previous<") {
		t.Errorf("empty page unexpectedly shows Previous link: %s", body)
	}
	if strings.Contains(body, ">Next<") {
		t.Errorf("empty page unexpectedly shows Next link: %s", body)
	}
}

// seedActionLog inserts a small set of action_log rows with known
// fields so the filter assertions are precise. The fixture seeds one
// row per "bucket" the filter tests care about:
//
//	alice/import/program   2026-05-01
//	bob/update/program     2026-05-15
//	system/login/session   2026-06-01
//	alice/deactivate/...   2026-06-15
//	alice/create/employer  2026-06-20
func seedActionLog(t *testing.T, db *sql.DB) {
	t.Helper()

	queries := storagedb.New(db)
	ctx := context.Background()

	type row struct {
		actor      string
		action     string
		entityType string
		entityID   sql.NullInt64
		details    string
		createdAt  string
	}
	rows := []row{
		{"alice", "import", "program", sql.NullInt64{Int64: 1, Valid: true}, `{"file":"a.xlsx"}`, "2026-05-01T10:00:00Z"},
		{"bob", "update", "program", sql.NullInt64{Int64: 2, Valid: true}, `{"from":1,"to":2}`, "2026-05-15T10:00:00Z"},
		{"system", "login", "session", sql.NullInt64{}, `{}`, "2026-06-01T10:00:00Z"},
		{"alice", "deactivate", "program_group", sql.NullInt64{Int64: 7, Valid: true}, `{"to":"inactive"}`, "2026-06-15T10:00:00Z"},
		{"alice", "create", "employer", sql.NullInt64{Int64: 42, Valid: true}, `{"inn":"7700"}`, "2026-06-20T10:00:00Z"},
	}
	for _, r := range rows {
		_, err := queries.CreateActionLog(ctx, storagedb.CreateActionLogParams{
			Actor:      r.actor,
			Action:     r.action,
			EntityType: r.entityType,
			EntityID:   r.entityID,
			Details:    sql.NullString{String: r.details, Valid: true},
			CreatedAt:  r.createdAt,
		})
		if err != nil {
			t.Fatalf("seed action_log %+v: %v", r, err)
		}
	}
}

// seedBulkActionLog inserts n action_log rows with deterministic,
// monotonically-increasing timestamps so pagination assertions can
// reason about ordering.
func seedBulkActionLog(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	queries := storagedb.New(db)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		ts := base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
		_, err := queries.CreateActionLog(ctx, storagedb.CreateActionLogParams{
			Actor:      "bulk",
			Action:     "create",
			EntityType: "program",
			EntityID:   sql.NullInt64{Int64: int64(i + 1), Valid: true},
			Details:    sql.NullString{String: `{"i":` + strconv.Itoa(i) + `}`, Valid: true},
			CreatedAt:  ts,
		})
		if err != nil {
			t.Fatalf("seed bulk %d: %v", i, err)
		}
	}
}

// authedGet issues an authenticated GET against the router. The
// HX-Request header is left empty; tests that exercise the HTMX path
// can set it explicitly.
func authedGet(t *testing.T, router http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "example.com"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

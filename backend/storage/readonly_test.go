package storage

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadOnlyDatabaseDSNBuildsPortableFileURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		wantHost    string
		wantPath    string
		wantEscaped string
	}{
		{
			name:        "Windows drive",
			path:        "C:/Program Data/IKC/app#1%?.db",
			wantPath:    "/C:/Program Data/IKC/app#1%?.db",
			wantEscaped: "/C:/Program%20Data/IKC/app%231%25%3F.db",
		},
		{
			name:        "UNC share",
			path:        "//server/share/IKC app.db",
			wantHost:    "server",
			wantPath:    "/share/IKC app.db",
			wantEscaped: "/share/IKC%20app.db",
		},
		{
			name:        "POSIX absolute",
			path:        "/var/lib/IKC app#1%?.db",
			wantPath:    "/var/lib/IKC app#1%?.db",
			wantEscaped: "/var/lib/IKC%20app%231%25%3F.db",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			dsn := readOnlyDatabaseDSN(test.path)
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("url.Parse(%q) error = %v", dsn, err)
			}
			if parsed.Scheme != "file" || parsed.Host != test.wantHost || parsed.Path != test.wantPath {
				t.Fatalf("parsed URI = scheme %q, host %q, path %q; want file, %q, %q", parsed.Scheme, parsed.Host, parsed.Path, test.wantHost, test.wantPath)
			}
			if got := parsed.EscapedPath(); got != test.wantEscaped {
				t.Fatalf("escaped path = %q, want %q", got, test.wantEscaped)
			}
			query := parsed.Query()
			if got := query.Get("mode"); got != "ro" {
				t.Fatalf("mode = %q, want ro", got)
			}
			wantPragmas := []string{"busy_timeout(5000)", "query_only(1)"}
			if got := query["_pragma"]; !reflect.DeepEqual(got, wantPragmas) {
				t.Fatalf("pragmas = %q, want %q", got, wantPragmas)
			}
		})
	}
}

func TestOpenReadOnlyPermitsReadsAndRejectsWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "readonly.db")
	writable, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := writable.ExecContext(ctx, `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := writable.ExecContext(ctx, `INSERT INTO items (name) VALUES ('existing')`); err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close writable database: %v", err)
	}

	readonly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("OpenReadOnly() error = %v", err)
	}
	defer readonly.Close()

	var name string
	if err := readonly.QueryRowContext(ctx, `SELECT name FROM items WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("SELECT error = %v", err)
	}
	if name != "existing" {
		t.Fatalf("SELECT name = %q, want existing", name)
	}
	if _, err := readonly.ExecContext(ctx, `INSERT INTO items (name) VALUES ('forbidden')`); err == nil {
		t.Fatal("INSERT error = nil, want read-only failure")
	}
}

func TestOpenReadOnlyDoesNotCreateMissingDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := OpenReadOnly(ctx, path); err == nil {
		t.Fatal("OpenReadOnly() error = nil, want missing database error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("os.Stat() error = %v, want file to remain missing", err)
	}
}

func TestOpenReadOnlyRejectsInvalidPaths(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"", "relative.db"} {
		if _, err := OpenReadOnly(context.Background(), path); err == nil {
			t.Fatalf("OpenReadOnly(%q) error = nil, want invalid path error", path)
		}
	}
}

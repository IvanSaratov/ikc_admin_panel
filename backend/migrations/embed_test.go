package migrations

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

func TestEmbeddedMigrationsContainOnlyReleaseBaseline(t *testing.T) {
	files, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatalf("glob embedded migrations: %v", err)
	}

	want := []string{"001_baseline.sql"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("embedded migrations = %v, want %v", files, want)
	}
}

func TestEmbeddedMigrationsExcludeUnsupportedGooseAnnotations(t *testing.T) {
	files, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatalf("glob embedded migrations: %v", err)
	}

	rejected := []string{
		"-- +GOOSE DOWN",
		"-- +GOOSE NO TRANSACTION",
		"-- +GOOSE ENVSUB ON",
	}
	for _, name := range files {
		data, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatalf("read embedded migration %s: %v", name, err)
		}
		for lineNumber, line := range strings.Split(string(data), "\n") {
			normalized := strings.Join(strings.Fields(strings.ToUpper(line)), " ")
			for _, annotation := range rejected {
				if normalized == annotation {
					t.Errorf("%s:%d: unsupported goose annotation %q", name, lineNumber+1, line)
				}
			}
		}
	}
}

func TestEmbeddedMigrationsBaselineSetsApplicationID(t *testing.T) {
	data, err := fs.ReadFile(FS, "001_baseline.sql")
	if err != nil {
		t.Fatalf("read embedded release baseline: %v", err)
	}

	const applicationID = "PRAGMA application_id = 0x494B4341;"
	if !strings.Contains(string(data), applicationID) {
		t.Fatalf("001_baseline.sql does not contain exact text %q", applicationID)
	}
}

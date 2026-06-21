# Legacy `mintrud_generator` Audit

**Date:** 2026-06-21
**Source repo:** https://github.com/IvanSaratov/mintrud_generator (private, cloned to `/tmp/mintrud-generator-audit/mintrud_generator/`)
**Purpose:** Identify reusable document-generation packages (XML, DOCX, XLSX, Moodle) for integration into Mintrud Admin Panel (`backend/documents/`).

---

## 1. Repository overview

**Go module:** `github.com/IvanSaratov/mintrud_generator`
**Min Go version:** 1.25.5 (per `go.mod`); README claims "Go 1.21+" — `go.mod` is authoritative.
**Entry point:** `mintrud_generator.go` at repo root (CLI bootstrap via `urfave/cli/v2`).

**Top-level layout:**
- `mintrud_generator.go` — CLI entry, wires `urfave/cli` subcommands (`serve`, `init-config`, ...).
- `src/core/` — shared helpers (config loading, logging via `logrus`, file output).
- `src/models/` — domain structs (organizations, workers, programs, protocols).
- `src/reader/` — XLSX input parsing (employee rosters from spreadsheets).
- `src/generator/` — output document generators: XML (Mintrud registry), DOCX (protocols).
- `src/moodle/` — Moodle REST client: user creation, course enrolment.
- `src/initiate/` — bootstrap helpers (DB-less; pure config init).
- `src/server/` — `gorilla/mux` HTTP server, embedded `resources/` (templates).
- `installer/` — WiX MSI build scripts (Windows-only).

**Build tags / platform constraints:**
- No explicit `//go:build` constraints in the generator packages themselves (verified via grep).
- No platform-specific files — code is portable across OSes.
- README notes Windows is required *only* for MSI installer build and running as a service; the generator libraries themselves cross-compile cleanly to macOS/Linux (already implied by the user's `worktree-agent-*` working on macOS).

**Direct deps of interest for our integration:**
- `github.com/gorilla/mux v1.8.1` — HTTP routing (we use Echo instead — replaceable).
- `github.com/urfave/cli/v2` — CLI bootstrap (irrelevant to library integration).
- `github.com/sirupsen/logrus v1.9.3` — logging (we use stdlib `log/slog`).
- `github.com/xuri/excelize/v2 v2.9.0` — XLSX read/write.
- `github.com/fumiama/go-docx v0.0.0-20240924153044-f7d29bb5c371` and `github.com/lukasjarosch/go-docx v0.5.0` — DOCX generation.
- `github.com/shabbyrobe/xmlwriter` — low-level XML emission for the Mintrud registry schema.
- `github.com/go-resty/resty/v2` — HTTP client (used by Moodle client).
- `github.com/mehanizm/iuliia-go`, `github.com/goodsign/monday`, `github.com/amonsat/fullname_parser` — text transliteration / date / FIO helpers.

---

## 2. XML package inventory

**Package path:** `github.com/IvanSaratov/mintrud_generator/src/generator` (single package covers XML + DOCX; XML logic lives in `gen_xml.go`).

**Domain types it consumes** (from `src/models`):
- `RegistrySet`, `RegistryRecord`, `Worker`, `Organization`, `Test` — all in `models/xml.go`.
- See exact field layout in the `xml:"..."` tags above.

**Public entry point:**

```go
// gen_xml.go:16
func GenerateXML(data *models.RegistrySet) ([]byte, error)
```

- **Input:** a `*models.RegistrySet` — a set of `RegistryRecord` entries (workers with embedded test/protocol data).
- **Output:** UTF-8 byte slice (XML document, 4-space indent, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`).
- **Errors:** propagated via `xmlwriter.ErrCollector`; returns the first error encountered (schema-shape or IO).
- **Schema quirks** (per inline comments): every element uses `Full: true` so empty fields render as `<Tag></Tag>`, not `<Tag/>` — the Mintrud validator rejects self-closing tags for empty optional fields (e.g. blank middle name). `isPassed` is hardcoded `true` because unpassed rows are filtered at XLSX-read time.

**External Go deps (this package only):**
- `github.com/shabbyrobe/xmlwriter` — streaming XML writer with deferred error collection.
- No other imports; pure stdlib `bytes` for the output buffer.

**Coupling notes:**
- The package has no DB / IO deps — fully unit-testable (see `gen_xml_test.go`).
- Adapter responsibility: build `*models.RegistrySet` from our domain (DB rows in `audit_protocols`, `audit_workers`, `audit_programs`, `audit_employers`) — see §7.
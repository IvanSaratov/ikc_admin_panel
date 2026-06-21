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

---

## 3. DOCX package inventory

**Package path:** same `github.com/IvanSaratov/mintrud_generator/src/generator` (file `gen_docx.go`). Function `CreateDocx` is exported alongside `GenerateXML`.

**Public entry point:**

```go
// gen_docx.go:35
func CreateDocx(
    data *models.RegistrySet,
    templatePath string,   // raw bytes of the DOCX template (NB: typed as string but used as []byte)
    timeType string,       // one of "А", "Б", "В", "П", "С" — program category for hours lookup
    log *logrus.Logger,
) ([][]byte, error)
```

- **Input:** `*models.RegistrySet` (same as XML), a DOCX template (3 tables: header, commission, participants), a program-type key, and a logger.
- **Output:** a slice of DOCX byte streams — one entry per `LearnProgramIdAttr` group, grouped by program and sorted by program ID for deterministic output ordering.
- **Output is *not* zipped** — caller is responsible for ZIP wrapping (the legacy server does this; we need to do the same in the adapter).

**Pipeline (worth understanding for integration):**

1. **Stage 1 — placeholder substitution** via `github.com/lukasjarosch/go-docx` (raw XML replacement of `{placeholder}` tokens). Placeholders used: `people_count`, `user_program`, `program_time`, `protocol_number`, `protocol_date`, `education_start`, `education_end`. Dates are Russian-locale-formatted via `monday`.
2. **Stage 2 — table-row insertion** via `github.com/fumiama/go-docx` (which can mutate OOXML structure but can't replace placeholders). Participants are appended to table index `2` (the third table, 8 columns: №, Организация, ФИО, Должность, Результат, Дата, Рег.номер, Подпись). Result column is hardcoded "удовл".
3. **Stage 3 — sectPr restoration** via custom ZIP patching in `restoreSectPr` (lines 245-286): `fumiama/go-docx` strips `w:orient` and `pgMar` from `<w:sectPr>` during parse, so the code surgically re-injects the original sectPr from the template after writing.

**External Go deps (DOCX path):**
- `github.com/lukasjarosch/go-docx v0.5.0` — placeholder substitution.
- `github.com/fumiama/go-docx v0.0.0-20240924153044-...` — table-row manipulation.
- `github.com/goodsign/monday v1.0.2` — Russian date formatting.
- `github.com/sirupsen/logrus v1.9.3` — logging.
- No DB, no HTTP — but requires the DOCX template as a runtime byte blob. **The template itself is not in the repo** — we must obtain the legacy `protocol.docx` from the legacy maintainer.

**Coupling notes / risks:**
- Hardcoded "Times New Roman" font strings everywhere (line 137-144) — any new template must use this font.
- `templatePath` parameter is misnamed: declared `string` but used as `[]byte`. We should rename to `template []byte` in the adapter wrapper.
- `logrus.Logger` is a public dep — we'd inject our own logger or replace the call sites.

---

## 4. XLSX package inventory

**Package path:** `github.com/IvanSaratov/mintrud_generator/src/reader` (file `xlsx.go`).

**Public entry point:**

```go
// xlsx.go:34
func ReadXLSX(
    fileContent *bytes.Buffer,        // uploaded XLSX file
    table string,                     // worksheet name
    positions string,                 // comma-separated row numbers, e.g. "1,3-5,8"
    programs []string,                // program IDs (matches models.LESSON_BY_NAME keys)
    org *models.Organization,         // default org stamped on every record
    log *logrus.Logger,
) (*models.RegistrySet, error)
```

- **Input:** XLSX byte stream + sheet name + 1-based row positions + program IDs.
- **Output:** a `*models.RegistrySet` ready for `GenerateXML` or `CreateDocx`.
- **Use case in legacy:** bulk-import mode via web UI (admin uploads XLSX, picks rows, picks programs, hits a button). **Our admin panel does not need this path** — workers, programs, and protocols are already in our DB (PostgreSQL). XLSX package is therefore **out of MVP scope**. Recorded for parity and possible future bulk-import feature.

**Column contract (from comments):**

| Col | Meaning                                          |
|-----|--------------------------------------------------|
| A   | Employer title                                   |
| B   | Employer INN                                     |
| C   | Full FIO (free-form, parsed by `fullname_parser`)|
| D   | Position                                         |
| F   | SNILS (blank = foreign worker)                   |
| G   | Protocol number                                  |
| H   | Protocol date (`1-2-06` → `2-1-2006`)            |
| J   | Education period (`01.01.2024-12.12.2024`)       |
| L   | Email (optional, warns if missing)              |

**External Go deps (XLSX path):**
- `github.com/xuri/excelize/v2 v2.9.0` — XLSX read/write.
- `github.com/amonsat/fullname_parser` — Russian FIO parser (note the **inverted** field semantics: `parsed.First` → LastName, `parsed.Middle` → FirstName, `parsed.Last` → FirstName or MiddleName; see xlsx.go:86-94 inline comment).
- `github.com/sirupsen/logrus v1.9.3` — logging.
- `github.com/IvanSaratov/mintrud_generator/src/core` — internal helper `ConvertStringToNumber` (parses "1,3-5,8" into `[]int`).

**Coupling notes / risks:**
- `fullname_parser` mapping quirk (above) — if D3 ever does need bulk-import, this must be preserved or re-tested.
- `core.ConvertStringToNumber` lives in the same module — can't be used as a standalone import if we go with `go.mod replace` (it pulls the whole `core` package, including `logrus` setup).
- Logic of "one record per (row, program)" (`xlsx.go:54-153`) is wasteful for our use case where DB rows are already 1:1 with workers per protocol.
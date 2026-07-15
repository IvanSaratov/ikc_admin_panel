// cmd/seed populates the IKC Expert SQLite database with a small,
// fully synthetic Russian-language fixture set designed to exercise every UI
// surface the Core MVP ships: directory browse + filter, request
// lifecycle (pending → applied), protocol lifecycle (draft → completed),
// and the audit log.
//
// Usage: go run ./cmd/seed /absolute/path/to/ikc.db
//
// The script is idempotent only in the trivial sense that running it
// twice will fail on UNIQUE indexes — by design. It is meant to be run
// against an empty (migrated) database, e.g. immediately after the
// docker compose stack starts for the first time.
//
// All inserts run inside one transaction so a partial seed can never
// leave the DB in a half-populated state. SQLite's CHECK constraints
// and FOREIGN KEY enforcement apply, so a typo in a status vocabulary
// will abort the whole seed with the offending row reported.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/platform/logging"
	"github.com/IvanSaratov/ikc_admin_panel/backend/storage"
	"go.uber.org/zap"
)

// now is the canonical "seed moment" — all created_at / updated_at
// values are anchored here so the seeded timeline reads coherently.
var now = time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)

var seedLog = zap.NewNop().Sugar()

func main() {
	logger, err := logging.New(logging.Config{
		Env:    os.Getenv("IKC_SEED_ENV"),
		Level:  os.Getenv("IKC_SEED_LOG_LEVEL"),
		Format: os.Getenv("IKC_SEED_LOG_FORMAT"),
		Output: os.Stdout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "FATAL: invalid log configuration")
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()
	seedLog = logger.Sugar()

	if len(os.Args) < 2 {
		seedLog.Fatalf("usage: %s <db-path>", os.Args[0])
	}
	ctx := context.Background()
	db, err := storage.Open(ctx, os.Args[1])
	if err != nil {
		seedLog.Fatalf("open db: %v", err)
	}
	defer db.Close()

	fx := buildFixture()
	if err := apply(ctx, db, fx); err != nil {
		seedLog.Fatalf("seed: %v", err)
	}
	seedLog.Infof("seed: complete (%d program_groups, %d programs, %d employers, %d workers, %d requests, %d protocols, %d action_log entries)",
		len(fx.programGroups), len(fx.programs), len(fx.employers),
		len(fx.workers), len(fx.requests), len(fx.protocols), len(fx.actionLog))
}

// --- fixture data (in-memory; no DB I/O) ------------------------------------

type workerSpec struct {
	last, first, middle, snils, email, birth, position string
	employerKey                                        string // e0 / e1 / e2
}

type requestRowSpec struct {
	workerKey  string // empty => not parsed
	programKey string
	status     string // request_rows.status
	errSummary string
	// when set, a matching request_training_items row is inserted.
	item *trainingItemSpec
}

type trainingItemSpec struct {
	programKey string
	status     string // request_training_items.status
	errSummary string
	resolution string // skip_duplicate / link_existing / create_repeat / ""
	// denormalised onto the linked training_record:
	workerKey    string
	position     string
	hours        int
	needsMintrud bool
}

type requestSpec struct {
	employerKey string
	received    time.Time
	source      string // xlsx / manual / other
	importKey   string // "" => no import link
	status      string // review / completed / cancelled
	notes       string
	rows        []requestRowSpec
}

type protocolSpec struct {
	groupKey      string
	status        string
	trainingStart string // YYYY-MM-DD or ""
	trainingEnd   string
	protocolDate  string
	sequenceYear  int64 // 0 => unset
	protocolMonth int64
	annualSeq     int64
	number        string
	fixedAt       time.Time // zero => unset
}

type generationRunSpec struct {
	protocolKey string
	typ         string
	status      string
	file        string
	errMsg      string
	generated   time.Time
}

type actionLogSpec struct {
	actor      string
	action     string
	entityType string
	entityKey  string // importedKey / requestKey / protocolKey / ""
	details    string
	at         time.Time
}

type fixture struct {
	// Static reference data
	programGroups []struct{ code, name string }
	programs      []struct {
		groupCode, code, name string
		hours                 int
	}

	// Employers: edge-case inactive marked via flag
	employers []struct {
		inn, name string
		inactive  bool
	}

	// Workers each carry their assigned employer for the worker_employers row
	workers []workerSpec

	// Imports: at least one failed (edge)
	imports []struct {
		source   string
		file     string
		sha      string
		uploaded string
		received time.Time
		status   string
	}

	requests  []requestSpec
	protocols []protocolSpec
	genRuns   []generationRunSpec
	actionLog []actionLogSpec
}

func buildFixture() *fixture {
	fx := &fixture{}

	// 2 program groups
	fx.programGroups = []struct{ code, name string }{
		{"OT", "Охрана труда"},
		{"PB", "Промышленная безопасность"},
	}

	// 5 programs (3 OT, 2 PB)
	fx.programs = []struct {
		groupCode, code, name string
		hours                 int
	}{
		{"OT", "OT-BASE", "Охрана труда в организациях", 16},
		{"OT", "OT-HEIGHT", "Охрана труда при работе на высоте", 40},
		{"OT", "OT-ELEC2", "Электробезопасность (II группа)", 72},
		{"PB", "PB-A1", "Промышленная безопасность А.1", 40},
		{"PB", "PB-B9", "Промышленная безопасность Б.9.31 (электроустановки)", 32},
	}

	// 3 employers; e2 is inactive (edge case — UI should still show but flag)
	fx.employers = []struct {
		inn, name string
		inactive  bool
	}{
		{"7701234567", `ООО "Учебный Металл"`, false},
		{"6623004567", `АО "Демо Завод"`, false},
		{"7204001234", `ООО "Пример Энерго"`, true},
	}

	// 15 synthetic workers, 5 per employer. Emails use RFC 2606 .example names.
	fx.workers = []workerSpec{
		{"Тестов", "Алексей", "Петрович", "112-233-445 95", "worker01@metal.example", "1985-03-12", "Слесарь-ремонтник 5 разряда", "e0"},
		{"Тестова", "Мария", "Сергеевна", "123-456-789 01", "worker02@metal.example", "1990-07-25", "Инженер-конструктор 2 категории", "e0"},
		{"Учебный", "Дмитрий", "Александрович", "234-567-890 12", "worker03@metal.example", "1978-11-04", "Электромонтёр 5 разряда", "e0"},
		{"Учебная", "Елена", "Викторовна", "345-678-901 23", "worker04@metal.example", "1988-01-18", "Бухгалтер по зарплате", "e0"},
		{"Демо", "Игорь", "Владимирович", "456-789-012 34", "worker05@metal.example", "1992-09-30", "Начальник ремонтного участка", "e0"},
		{"Примерова", "Анна", "Михайловна", "567-890-123 45", "worker06@plant.example", "1983-05-22", "Токарь 4 разряда", "e1"},
		{"Примеров", "Сергей", "Николаевич", "678-901-234 56", "worker07@plant.example", "1995-02-14", "Электросварщик ручной дуговой сварки", "e1"},
		{"Фиктивная", "Ольга", "Дмитриевна", "789-012-345 67", "worker08@plant.example", "1989-12-08", "Мастер смены", "e1"},
		{"Фиктивный", "Андрей", "Иванович", "890-123-456 78", "worker09@plant.example", "1981-06-17", "Машинист крана 6 разряда", "e1"},
		{"Макетова", "Татьяна", "Александровна", "901-234-567 89", "worker10@plant.example", "1996-08-03", "Инженер по охране труда", "e1"},
		{"Макетов", "Михаил", "Юрьевич", "012-345-678 90", "worker11@energy.example", "1987-04-26", "Оператор котельной установки", "e2"},
		{"Сценарная", "Наталья", "Сергеевна", "135-246-357 91", "worker12@energy.example", "1991-10-11", "Технолог нефтепереработки", "e2"},
		{"Сценарный", "Павел", "Алексеевич", "246-357-468 02", "worker13@energy.example", "1979-01-29", "Электрик 5 разряда", "e2"},
		{"Образцова", "Ирина", "Викторовна", "357-468-579 13", "worker14@energy.example", "1993-03-07", "Лаборант химического анализа", "e2"},
		{"Образцов", "Артём", "Олегович", "468-579-680 24", "worker15@energy.example", "1986-11-22", "Инженер-механик", "e2"},
	}

	// 2 imports; one failed (edge case)
	fx.imports = []struct {
		source   string
		file     string
		sha      string
		uploaded string
		received time.Time
		status   string
	}{
		{"xlsx", "sample_request_2026-05-12.xlsx",
			"9f1c0a4e2b8d7f3a1c5e6b2d4f8a9c0e1b3d5f7a9c2e4b6d8f0a1c3e5b7d9f1a",
			"admin", ago(40 * 24 * time.Hour), "completed"},
		{"manual", "", "", "admin", ago(7 * 24 * time.Hour), "failed"},
	}

	// 5 requests with deliberately varied lifecycle states
	fx.requests = []requestSpec{
		{
			employerKey: "e0",
			received:    ago(40 * 24 * time.Hour),
			source:      "xlsx",
			importKey:   "im0",
			status:      "completed",
			notes:       "Заявка ОТ-службы на майскую сессию",
			rows: []requestRowSpec{
				{workerKey: "w0", programKey: "OT-BASE", status: "applied",
					item: &trainingItemSpec{programKey: "OT-BASE", status: "applied",
						workerKey: "w0", position: "Слесарь-ремонтник 5 разряда", hours: 16, needsMintrud: true}},
				{workerKey: "w1", programKey: "OT-BASE", status: "applied",
					item: &trainingItemSpec{programKey: "OT-BASE", status: "applied",
						workerKey: "w1", position: "Инженер-конструктор 2 категории", hours: 16, needsMintrud: true}},
				{workerKey: "w2", programKey: "OT-ELEC2", status: "applied",
					item: &trainingItemSpec{programKey: "OT-ELEC2", status: "applied",
						workerKey: "w2", position: "Электромонтёр 5 разряда", hours: 72, needsMintrud: true}},
			},
		},
		{
			employerKey: "e1",
			received:    ago(28 * 24 * time.Hour),
			source:      "xlsx",
			importKey:   "im0",
			status:      "review",
			notes:       "Срочно: переаттестация перед аудитом",
			rows: []requestRowSpec{
				{workerKey: "w5", programKey: "PB-A1", status: "applied",
					item: &trainingItemSpec{programKey: "PB-A1", status: "applied",
						workerKey: "w5", position: "Токарь 4 разряда", hours: 40, needsMintrud: true}},
				{workerKey: "w6", programKey: "PB-B9", status: "pending"},
				{workerKey: "w7", programKey: "PB-A1", status: "invalid",
					errSummary: "СНИЛС не прошёл контрольную сумму"},
			},
		},
		{
			employerKey: "e0",
			received:    ago(20 * 24 * time.Hour),
			source:      "manual",
			status:      "review",
			notes:       "Документы принесли лично, 1 строка нечитаема",
			rows: []requestRowSpec{
				{workerKey: "", programKey: "OT-BASE", status: "pending"},
				{workerKey: "", programKey: "OT-HEIGHT", status: "invalid",
					errSummary: "Нечитаемое отчество, строка 2"},
				{workerKey: "w3", programKey: "OT-HEIGHT", status: "applied",
					item: &trainingItemSpec{programKey: "OT-HEIGHT", status: "applied",
						workerKey: "w3", position: "Бухгалтер по зарплате", hours: 40, needsMintrud: false}},
			},
		},
		{
			employerKey: "e0",
			received:    ago(10 * 24 * time.Hour),
			source:      "xlsx",
			importKey:   "im0",
			status:      "review",
			notes:       "Повторная заявка, ФИО дублируется",
			rows: []requestRowSpec{
				{workerKey: "w4", programKey: "OT-BASE", status: "applied",
					item: &trainingItemSpec{programKey: "OT-BASE", status: "applied",
						workerKey: "w4", position: "Начальник ремонтного участка", hours: 16, needsMintrud: true}},
				{workerKey: "w4", programKey: "OT-BASE", status: "skipped",
					errSummary: "Дубликат: тот же СНИЛС в строке 1",
					item: &trainingItemSpec{programKey: "OT-BASE", status: "skipped",
						resolution: "skip_duplicate"}},
			},
		},
		{
			employerKey: "e1",
			received:    ago(26 * 24 * time.Hour),
			source:      "manual",
			status:      "cancelled",
			notes:       "Отменена заказчиком после выгрузки",
			rows: []requestRowSpec{
				{workerKey: "w9", programKey: "OT-HEIGHT", status: "pending"},
			},
		},
	}

	// 2 protocols: 1 completed (full lifecycle), 1 draft
	fx.protocols = []protocolSpec{
		{
			groupKey: "OT", status: "completed",
			trainingStart: "2026-05-15",
			trainingEnd:   "2026-05-29",
			protocolDate:  "2026-05-29",
			sequenceYear:  2026, protocolMonth: 5, annualSeq: 1,
			number:  "ОТ-2026/1",
			fixedAt: ago(45 * 24 * time.Hour),
		},
		{
			groupKey: "PB", status: "draft",
		},
	}

	// 4 generation runs
	fx.genRuns = []generationRunSpec{
		{"p0", "xml", "success", "OT-2026-1.xml", "", ago(28 * 24 * time.Hour)},
		{"p0", "docx", "success", "OT-2026-1.docx", "", ago(27 * 24 * time.Hour)},
		{"p0", "moodle_credentials", "failed", "", "moodle sync disabled (mintrud_generator feature flag off)", ago(26 * 24 * time.Hour)},
		{"p1", "xml", "stale", "", "protocol not yet fixed", ago(2 * 24 * time.Hour)},
	}

	// 9 action_log entries spanning every actor type
	fx.actionLog = []actionLogSpec{
		{"system", "db_migrate", "system", "",
			"released baseline schema applied", ago(45 * 24 * time.Hour)},
		{"import", "xlsx_import_received", "import", "im0",
			"operator uploaded sample_request_2026-05-12.xlsx (sha256 matches)", ago(40 * 24 * time.Hour)},
		{"import", "xlsx_rows_applied", "client_request", "r0",
			"3 rows applied, 0 invalid, 0 skipped", ago(40*24*time.Hour + 30*time.Minute)},
		{"import", "xlsx_import_failed", "import", "im1",
			"manual entry rejected: missing required INN field on row 1", ago(7 * 24 * time.Hour)},
		{"operator_unidentified", "legacy_import_run", "import", "im0",
			"imported by mintrud_generator pre-migration; operator identity not preserved", ago(45 * 24 * time.Hour)},
		{"admin", "protocol_fixed", "protocol", "p0",
			"assigned number ОТ-2026/1", ago(44 * 24 * time.Hour)},
		{"admin", "xml_generated", "protocol", "p0",
			"XML payload OT-2026-1.xml written", ago(28 * 24 * time.Hour)},
		{"admin", "registry_entered", "protocol", "p0",
			"4 participants entered in Минтруд registry", ago(35 * 24 * time.Hour)},
		{"system", "moodle_sync_failed", "protocol", "p0",
			"moodle integration disabled (out of MVP scope); generation_run marked failed", ago(26 * 24 * time.Hour)},
	}

	return fx
}

// --- apply (DB I/O in dependency order) --------------------------------------

type ids struct {
	programGroup map[string]int64 // code -> id
	program      map[string]int64 // code -> id
	employer     map[string]int64 // eN -> id
	worker       map[string]int64 // wN -> id
	we           map[string]int64 // wN -> id (one-to-one with worker)
	imp          map[string]int64 // imN -> id
	request      map[string]int64 // rN -> id
	requestRow   map[int]int64    // row index (within request) -> id
	training     []int64          // ordered, matching trainingItemSpec order
	protocol     map[string]int64 // pN -> id
}

func newIDs() *ids {
	return &ids{
		programGroup: map[string]int64{},
		program:      map[string]int64{},
		employer:     map[string]int64{},
		worker:       map[string]int64{},
		we:           map[string]int64{},
		imp:          map[string]int64{},
		request:      map[string]int64{},
		requestRow:   map[int]int64{},
		protocol:     map[string]int64{},
	}
}

func apply(ctx context.Context, db *sql.DB, fx *fixture) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ids := newIDs()

	for i, g := range fx.programGroups {
		id := mustExec(ctx, tx, `
			INSERT INTO program_groups (code, name, status, created_at, updated_at)
			VALUES (?, ?, 'active', ?, ?)`,
			g.code, g.name, ts(ago(720*time.Hour)), ts(ago(720*time.Hour)))
		ids.programGroup[g.code] = id
		seedLog.Infof("seed: program_group %s -> %d", g.code, id)
		if i == len(fx.programGroups)-1 {
			seedLog.Infof("seed: program_groups -> %d rows", len(fx.programGroups))
		}
	}

	for _, p := range fx.programs {
		id := mustExec(ctx, tx, `
			INSERT INTO programs
				(program_group_id, code, name, default_hours, moodle_course_id, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, NULL, 'active', ?, ?)`,
			ids.programGroup[p.groupCode], p.code, p.name, p.hours,
			ts(ago(720*time.Hour)), ts(ago(720*time.Hour)))
		ids.program[p.code] = id
	}
	seedLog.Infof("seed: programs -> %d rows", len(fx.programs))

	for i, e := range fx.employers {
		status := "active"
		if e.inactive {
			status = "inactive"
		}
		key := fmt.Sprintf("e%d", i)
		id := mustExec(ctx, tx, `
			INSERT INTO employers (inn, inn_normalized, canonical_name, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			e.inn, digitsOnly(e.inn), e.name, status,
			ts(ago(360*time.Hour)), ts(ago(48*time.Hour)))
		ids.employer[key] = id
	}
	seedLog.Infof("seed: employers -> %d rows (1 inactive edge case)", len(fx.employers))

	for i, w := range fx.workers {
		key := fmt.Sprintf("w%d", i)
		wid := mustExec(ctx, tx, `
			INSERT INTO workers
				(last_name, first_name, middle_name, snils, snils_normalized, email, birth_date, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)`,
			w.last, w.first, w.middle, w.snils, digitsOnly(w.snils), w.email, w.birth,
			ts(ago(300*time.Hour)), ts(ago(96*time.Hour)))
		ids.worker[key] = wid

		weID := mustExec(ctx, tx, `
			INSERT INTO worker_employers
				(worker_id, employer_id, current_position, status, created_at, updated_at)
			VALUES (?, ?, ?, 'active', ?, ?)`,
			wid, ids.employer[w.employerKey], w.position,
			ts(ago(300*time.Hour)), ts(ago(96*time.Hour)))
		ids.we[key] = weID
	}
	seedLog.Infof("seed: workers -> %d rows; worker_employers -> %d rows",
		len(fx.workers), len(fx.workers))

	for i, im := range fx.imports {
		key := fmt.Sprintf("im%d", i)
		id := mustExec(ctx, tx, `
			INSERT INTO imports
				(source_type, source_file_name, source_sha256, uploaded_by_actor, received_at, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			im.source,
			nullStr(im.file), nullStr(im.sha),
			im.uploaded,
			ts(im.received), im.status,
			ts(im.received), ts(im.received.Add(time.Hour)))
		ids.imp[key] = id
	}
	seedLog.Infof("seed: imports -> %d rows (1 failed edge case)", len(fx.imports))

	// requests, request_rows, request_training_items, training_records — interleaved
	// because we need request_row IDs to build training_items, and training_record
	// IDs to back-link from training_items.
	trainingCount := 0
	for ri, r := range fx.requests {
		rkey := fmt.Sprintf("r%d", ri)
		var importID sql.NullInt64
		if r.importKey != "" {
			id, ok := ids.imp[r.importKey]
			if !ok {
				return fmt.Errorf("request %s: unknown import %q", rkey, r.importKey)
			}
			importID = sql.NullInt64{Int64: id, Valid: true}
		}
		reqID := mustExec(ctx, tx, `
			INSERT INTO client_requests
				(employer_id, received_date, source_type, source_import_id, status, notes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ids.employer[r.employerKey], r.received.Format("2006-01-02"),
			r.source, importID, r.status, nullStr(r.notes),
			ts(r.received), ts(r.received.Add(2*time.Hour)))
		ids.request[rkey] = reqID

		for rowIdx, rs := range r.rows {
			rrID := insertRequestRow(ctx, tx, reqID, rowIdx+1, rs, fx)
			ids.requestRow[ri*100+rowIdx] = rrID

			if rs.item != nil {
				// Skipped items deliberately leave workerKey empty: the
				// request_training_items row carries the skip resolution
				// but no training_record is ever created.
				var trID sql.NullInt64
				if rs.item.workerKey != "" {
					id := insertTrainingRecord(ctx, tx, ids, rs.item)
					trID = sql.NullInt64{Int64: id, Valid: true}
				}
				itemID := mustExec(ctx, tx, `
					INSERT INTO request_training_items
						(request_row_id, program_id, status, error_summary, resolution, training_record_id, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					rrID, ids.program[rs.item.programKey],
					rs.item.status,
					nullStr(rs.item.errSummary),
					nullStr(rs.item.resolution),
					trID,
					ts(ago(30*24*time.Hour)), ts(ago(30*24*time.Hour)))
				if trID.Valid {
					ids.training = append(ids.training, trID.Int64)
					trainingCount++
				}
				_ = itemID
			}
		}
	}
	seedLog.Infof("seed: client_requests -> %d rows", len(fx.requests))
	seedLog.Infof("seed: request_training_items -> %d rows; training_records -> %d rows",
		trainingCount, trainingCount)

	for pi, p := range fx.protocols {
		pkey := fmt.Sprintf("p%d", pi)
		var (
			trainingStart = nullStr(p.trainingStart)
			trainingEnd   = nullStr(p.trainingEnd)
			protocolDate  = nullStr(p.protocolDate)
			number        = nullStr(p.number)
			fixedAt       sql.NullString
			seqYear       sql.NullInt64
			month         sql.NullInt64
			annualSeq     sql.NullInt64
		)
		if p.sequenceYear != 0 {
			seqYear = sql.NullInt64{Int64: p.sequenceYear, Valid: true}
			month = sql.NullInt64{Int64: p.protocolMonth, Valid: true}
			annualSeq = sql.NullInt64{Int64: p.annualSeq, Valid: true}
		}
		if !p.fixedAt.IsZero() {
			fixedAt = sql.NullString{String: ts(p.fixedAt), Valid: true}
		}
		id := mustExec(ctx, tx, `
			INSERT INTO protocols
				(program_group_id, status, training_start_date, training_end_date, protocol_date,
				 sequence_year, protocol_month, annual_sequence_number, protocol_number, protocol_suffix,
				 fixed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)`,
			ids.programGroup[p.groupKey], p.status,
			trainingStart, trainingEnd, protocolDate,
			seqYear, month, annualSeq,
			number, fixedAt,
			ts(ago(60*24*time.Hour)), ts(ago(2*24*time.Hour)))
		ids.protocol[pkey] = id
	}
	seedLog.Infof("seed: protocols -> %d rows (1 completed, 1 draft)", len(fx.protocols))

	// Attach first 4 training records to the completed protocol
	completedID := ids.protocol["p0"]
	for i := 0; i < 4 && i < len(ids.training); i++ {
		confirmedAt := ts(ago(35 * 24 * time.Hour))
		mustExec(ctx, tx, `
			INSERT INTO protocol_participants
				(protocol_id, training_record_id, status,
				 requires_mintrud_test_confirmed_at, mintrud_registry_number, mintrud_registry_entered_at,
				 created_at, updated_at)
			VALUES (?, ?, 'active', ?, ?, ?, ?, ?)`,
			completedID, ids.training[i],
			confirmedAt,
			fmt.Sprintf("РМТ-2026/05-%03d", i+1),
			confirmedAt,
			ts(ago(30*24*time.Hour)), ts(ago(30*24*time.Hour)))
	}
	seedLog.Infof("seed: protocol_participants -> 4 rows")

	for _, gr := range fx.genRuns {
		mustExec(ctx, tx, `
			INSERT INTO generation_runs
				(protocol_id, type, status, file_name, error_message, generated_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ids.protocol[gr.protocolKey], gr.typ, gr.status,
			nullStr(gr.file), nullStr(gr.errMsg),
			ts(gr.generated), ts(gr.generated))
	}
	seedLog.Infof("seed: generation_runs -> %d rows", len(fx.genRuns))

	for _, a := range fx.actionLog {
		var entityID sql.NullInt64
		switch a.entityType {
		case "import":
			if v, ok := ids.imp[a.entityKey]; ok {
				entityID = sql.NullInt64{Int64: v, Valid: true}
			}
		case "client_request":
			if v, ok := ids.request[a.entityKey]; ok {
				entityID = sql.NullInt64{Int64: v, Valid: true}
			}
		case "protocol":
			if v, ok := ids.protocol[a.entityKey]; ok {
				entityID = sql.NullInt64{Int64: v, Valid: true}
			}
		}
		mustExec(ctx, tx, `
			INSERT INTO action_log (actor, action, entity_type, entity_id, details, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			a.actor, a.action, a.entityType, entityID, nullStr(a.details), ts(a.at))
	}
	seedLog.Infof("seed: action_log -> %d rows", len(fx.actionLog))

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func insertRequestRow(ctx context.Context, tx *sql.Tx, reqID int64, rowNum int, rs requestRowSpec, fx *fixture) int64 {
	var (
		rawFullName sql.NullString
		parsedLast  sql.NullString
		parsedFirst sql.NullString
		parsedMid   sql.NullString
		parsedSNILS sql.NullString
		parsedEmail sql.NullString
		parsedPos   sql.NullString
	)
	if rs.workerKey != "" {
		// Find the matching worker fixture by key
		var w *workerSpec
		for i := range fx.workers {
			if fmt.Sprintf("w%d", i) == rs.workerKey {
				w = &fx.workers[i]
				break
			}
		}
		if w == nil {
			seedLog.Fatalf("request_row: unknown workerKey %q", rs.workerKey)
		}
		rawFullName = sql.NullString{String: w.last + " " + w.first + " " + w.middle, Valid: true}
		parsedLast = sql.NullString{String: w.last, Valid: true}
		parsedFirst = sql.NullString{String: w.first, Valid: true}
		parsedMid = sql.NullString{String: w.middle, Valid: true}
		parsedSNILS = sql.NullString{String: digitsOnly(w.snils), Valid: true}
		parsedEmail = sql.NullString{String: w.email, Valid: true}
		if rs.item != nil {
			parsedPos = sql.NullString{String: rs.item.position, Valid: true}
		}
	}
	return mustExec(ctx, tx, `
		INSERT INTO request_rows
			(client_request_id, row_number, raw_data, raw_full_name,
			 parsed_last_name, parsed_first_name, parsed_middle_name, parsed_snils, parsed_email, parsed_position,
			 status, error_summary, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		reqID, rowNum,
		fmt.Sprintf("raw_line_%d", rowNum),
		rawFullName, parsedLast, parsedFirst, parsedMid, parsedSNILS, parsedEmail, parsedPos,
		rs.status, nullStr(rs.errSummary),
		ts(ago(40*24*time.Hour)), ts(ago(40*24*time.Hour)))
}

func insertTrainingRecord(ctx context.Context, tx *sql.Tx, ids *ids, item *trainingItemSpec) int64 {
	return mustExec(ctx, tx, `
		INSERT INTO training_records
			(worker_employer_id, program_id, client_request_id, position, hours,
			 requires_mintrud_test, moodle_status, status, created_at, updated_at)
		VALUES (?, ?, NULL, ?, ?, ?, 'not_required', 'active', ?, ?)`,
		ids.we[item.workerKey], ids.program[item.programKey],
		item.position, item.hours, item.needsMintrud,
		ts(ago(30*24*time.Hour)), ts(ago(30*24*time.Hour)))
}

// --- helpers ----------------------------------------------------------------

func ts(t time.Time) string         { return t.UTC().Format(time.RFC3339) }
func ago(d time.Duration) time.Time { return now.Add(-d) }

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// mustExec runs a statement and panics with row-affected data on failure.
// A seed run is meant to be all-or-nothing; aborting fast on any error
// keeps the transaction rollback path simple.
func mustExec(ctx context.Context, tx *sql.Tx, query string, args ...any) int64 {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		seedLog.Fatalf("exec %q: %v\nargs: %#v", query, err, args)
	}
	id, _ := res.LastInsertId()
	return id
}

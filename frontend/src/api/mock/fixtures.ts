import type {
  AuditEvent,
  ClientRequest,
  Employer,
  GenerationRun,
  ImportRow,
  ImportRun,
  MoodleAccount,
  Program,
  ProtocolWorkflow,
  Worker
} from "./types";

export const requests: ClientRequest[] = [
  {
    id: "request-1",
    employerId: "employer-1",
    employerName: "ООО Тест-Сервис",
    receivedDate: "2026-07-08",
    status: "review",
    rowsTotal: 18,
    rowsNeedReview: 7,
    nextAction: "Разобрать конфликты импорта",
    attention: "danger"
  },
  {
    id: "request-2",
    employerId: "employer-2",
    employerName: "АО Пример-Проект",
    receivedDate: "2026-07-09",
    status: "ready",
    rowsTotal: 11,
    rowsNeedReview: 0,
    nextAction: "Сформировать протокол",
    attention: "normal"
  }
];

export const importRuns: ImportRun[] = [
  {
    id: "import-1",
    requestId: "request-1",
    fileName: "client-request-test-service.xlsx",
    uploadedAt: "2026-07-08 11:24",
    rowsTotal: 18,
    status: "review"
  }
];

export const importRows: ImportRow[] = [
  {
    id: "row-1",
    importId: "import-1",
    rowNumber: 2,
    fullName: "Иванов Иван Иванович",
    snils: "000-000-001 00",
    email: "ivanov@example.test",
    employerName: "ООО Тест-Сервис",
    position: "Инженер",
    programs: ["А-1", "П-1"],
    status: "new",
    reason: "Новая строка готова к применению"
  },
  {
    id: "row-2",
    importId: "import-1",
    rowNumber: 3,
    fullName: "Петров Петр Петрович",
    snils: "000-000-002 00",
    email: "petrov@example.test",
    employerName: "ООО Тест-Сервис",
    position: "Мастер участка",
    programs: ["А-1"],
    status: "conflict",
    reason: "СНИЛС совпал, ФИО отличается от существующей карточки"
  },
  {
    id: "row-3",
    importId: "import-1",
    rowNumber: 4,
    fullName: "Сидорова Анна Сергеевна",
    snils: "000-000-003 00",
    email: "training@example.test",
    employerName: "ООО Тест-Сервис",
    position: "Специалист",
    programs: ["В-1"],
    status: "duplicate",
    reason: "Точная копия ранее импортированной строки"
  }
];

export const protocols: ProtocolWorkflow[] = [
  {
    id: "protocol-2605-a-15",
    number: "2605А15",
    employerName: "ООО Тест-Сервис",
    programGroup: "А",
    period: "2026-07-01 - 2026-07-05",
    participants: 18,
    stages: [
      { id: "participants", label: "Участники", state: "done" },
      { id: "fix", label: "Фиксация", state: "done" },
      { id: "xml", label: "XML", state: "active" },
      { id: "registry", label: "Реестровые номера", state: "blocked", reason: "Заполните номера Минтруда для 3 участников" },
      { id: "docx", label: "DOCX", state: "pending" },
      { id: "closed", label: "Закрытие", state: "pending" }
    ]
  }
];

export const workers: Worker[] = [
  {
    id: "worker-1",
    fullName: "Иванов Иван Иванович",
    snils: "000-000-001 00",
    email: "ivanov@example.test",
    employerName: "ООО Тест-Сервис",
    position: "Инженер",
    activeTrainings: 2
  },
  {
    id: "worker-2",
    fullName: "Петров Петр Петрович",
    snils: "000-000-002 00",
    email: "petrov@example.test",
    employerName: "АО Пример-Проект",
    position: "Мастер участка",
    activeTrainings: 1
  }
];

export const employers: Employer[] = [
  { id: "employer-1", name: "ООО Тест-Сервис", inn: "0000000000", status: "active", requests: 3, workers: 18 },
  { id: "employer-2", name: "АО Пример-Проект", inn: "0000000001", status: "active", requests: 2, workers: 11 }
];

export const programs: Program[] = [
  { id: "program-a-1", groupCode: "А", code: "А-1", name: "Общие вопросы охраны труда", defaultHours: 40, status: "active", moodleCourseId: "course-example-a1" },
  { id: "program-b-1", groupCode: "Б", code: "Б-1", name: "Безопасные методы работ", defaultHours: 16, status: "active" },
  { id: "program-v-1", groupCode: "В", code: "В-1", name: "Работы повышенной опасности", defaultHours: 24, status: "active" },
  { id: "program-p-1", groupCode: "П", code: "П-1", name: "Первая помощь", defaultHours: 16, status: "active" },
  { id: "program-s-1", groupCode: "С", code: "С-1", name: "Средства индивидуальной защиты", defaultHours: 8, status: "active" }
];

export const generationRuns: GenerationRun[] = [
  { id: "run-1", type: "xml", status: "success", relatedEntity: "2605А15", fileName: "2605A15.xml", generatedAt: "2026-07-10 14:20" },
  { id: "run-2", type: "docx", status: "needs_rebuild", relatedEntity: "2605А15", fileName: "2605A15.zip", generatedAt: "2026-07-10 15:12" }
];

export const moodleAccounts: MoodleAccount[] = [
  { id: "moodle-1", workerName: "Иванов Иван Иванович", employerName: "ООО Тест-Сервис", email: "ivanov@example.test", status: "enrolled", course: "А-1" },
  { id: "moodle-2", workerName: "Петров Петр Петрович", employerName: "АО Пример-Проект", email: "petrov@example.test", status: "review_required", course: "Б-1" }
];

export const auditEvents: AuditEvent[] = [
  { id: "audit-1", at: "2026-07-10 15:18", actor: "operator_unidentified", action: "import.row.conflict", entity: "row-2", details: "СНИЛС совпал, ФИО отличается" },
  { id: "audit-2", at: "2026-07-10 15:22", actor: "system", action: "protocol.xml.generated", entity: "2605А15", details: "XML сформирован из mock workflow" }
];

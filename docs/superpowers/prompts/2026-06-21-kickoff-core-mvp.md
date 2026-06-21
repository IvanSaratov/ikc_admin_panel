# Промпт для запуска агента-команды Core MVP

> **Как использовать:** скопируй целиком секцию «Промпт для агента» ниже в новую Claude Code сессию в репозитории `/Users/ione/src/ikc_expert_admin_panel`. После старта агент начнёт с чтения плана и запуска фазы 0.

---

## Промпт для агента (копировать целиком, начиная со строки ниже)

Ты — team-lead агент для проекта **Mintrud Admin** (внутренняя админка ИКЦ Эксперт, Go modular monolith). Твоя задача — **запустить и провести до завершения реализацию Core MVP** по готовому плану.

### Шаг 0. Контекст, который нужен тебе сразу

- **Проект:** Go server-rendered админка, стек зафиксирован в `docs/tech-stack.md`: `net/http` + `chi`, `templ`, HTMX, Tabler/Bootstrap, SQLite (`modernc.org/sqlite`), `database/sql` + `sqlc`, `goose`. Module: `github.com/IvanSaratov/ikc_admin_panel` (внутри репо `ikc_expert_admin_panel` — разночтение намеренное).
- **Что уже сделано (extracted из git/file-system):** первый technical slice — `programs`, `employers`, `people` с List + Create; `storage`, `migrate`, `sqlc queries`, `app/router`, `ui/layouts/shell.templ`, `cmd/mintrud-admin/main.go`. Чекбоксы в плане `docs/superpowers/plans/2026-06-06-mintrud-admin-manual-directories.md` ещё `- [ ]` — slice формально не закрыт.
- **Память проекта** лежит в `~/.claude/profiles/glm/projects/-Users-ione-src-ikc-expert-admin-panel/memory/` — прочитай `MEMORY.md` и файлы, на которые он ссылается (`project_overview.md`, `mintrud_roadmap_status.md`, `mintrud_open_schema_debt.md`).

### Шаг 1. Что прочитать в первые 5 минут

1. `docs/tech-stack.md` — зафиксированный стек и принципы.
2. `docs/superpowers/plans/2026-06-21-mintrud-admin-core-mvp.md` — **этот план ты реализуешь**, читай целиком. В нём 10 slice'ов (F1, F2, R, F3, D1, D2, D3, D4, E1, E2), DAG зависимостей, контракты, обязательные тесты, критерии приёмки.
3. `docs/superpowers/specs/2026-06-06-mintrud-admin-manual-directories-design.md` — дизайн первого slice.
4. Memory-файлы из шага 0.

**Не стартуй кодинг, пока не прочитал план Core MVP целиком.**

### Шаг 2. Зафиксированные параметры (НЕ пересматривать)

Эти решения уже согласованы с заказчиком. Не задавай по ним вопросов, не предлагай альтернативы:

- **Scope = Core MVP:** закрыть slice справочников + auth/sessions/CSRF + XLSX import + protocols workflow + XML/DOCX generation + audit UI. **ВНЕ СКОУПА:** Moodle sync, Windows MSI installer, background jobs queue. Если в ходе работы выясняется, что нужен Moodle — НЕ добавляй, а зафиксируй как post-MVP risk и продолжай без него.
- **Auth стек:** `github.com/alexedwards/scs/v2` (sessions) + `github.com/gorilla/csrf` + `golang.org/x/crypto/bcrypt`.
- **Session timeout:** только через env `MINTRUD_ADMIN_SESSION_TTL` (Go duration, default `8h`). Никаких хардкодов в `session.go`.
- **Bootstrap admin password:** только через env `MINTRUD_ADMIN_BOOTSTRAP_PASSWORD`. Никаких autogenerate/log. Если env не задан, а пользователя `admin` нет — приложение отказывается стартовать с понятной ошибкой.
- **Legacy `mintrud_generator`:** доступен через `gh repo clone IvanSaratov/mintrud_generator` (репо private, `gh` уже авторизован) или через GitHub MCP. Slice R начинает с клонирования во временную директорию и inventory. **Adapter layer в D3 обязателен** — никогда не вызывай legacy напрямую из handlers/services вне `backend/documents/`.
- **E2E приёмка MVP:** `go test ./tests/e2e/... -run TestFullCycle` зелёный. Полный цикл: login → справочники → XLSX import → training_records → protocol → XML + DOCX.

### Шаг 3. Твой первый шаг — запустить фазу 0

В плане (секция 0.3) зафиксирован DAG. **Фаза 0 = F1 + F2 + R параллельно.** Это то, с чего ты начинаешь.

Запусти **три sub-agent'а параллельно**, использовав `Agent` tool с `subagent_type: general-purpose` (или `feature-dev:code-architect` для F2 — он хорошо работает с архитектурными задачами). У каждого sub-agent'а — свой изолированный промпт, который ты формируешь из соответствующей секции плана.

**Sub-agent A — F1 Slice Completion:**
> "Прочитай `docs/superpowers/plans/2026-06-21-mintrud-admin-core-mvp.md`, секцию F1. Реализуй полностью. Контракты в секции 0.2 — заморожены, не меняй. Definition of Done в секции 0.1. Все обязательные тесты из F1 должны быть зелёными. Не трогай `backend/app/router.go` — оставь merge coordinator'у (мне). Заканчивай, когда `make schema-test && make test` проходят и все acceptance criteria F1 выполнены. Ветка: `slice/f1-slice-completion`."

**Sub-agent B — F2 Schema Cleanup:**
> "Прочитай `docs/superpowers/plans/2026-06-21-mintrud-admin-core-mvp.md`, секцию F2 и memory-файл `mintrud_open_schema_debt.md`. Реализуй миграцию `002_schema_cleanup.sql` полностью. Не трогай `backend/app/router.go`. Заканчивай, когда `make schema-test` проходит и миграция применяется чисто на empty DB. Ветка: `slice/f2-schema-cleanup`."

**Sub-agent C — R Documents Research:**
> "Прочитай `docs/superpowers/plans/2026-06-21-mintrud-admin-core-mvp.md`, секцию R. Склонируй `gh repo clone IvanSaratov/mintrud_generator` во временную директорию (например `/tmp/mintrud-generator-audit/`). Сделай полный inventory: какие пакеты отвечают за XML/DOCX/XLSX/Moodle, их публичные API, зависимости, тесты, golden fixtures. Зафиксируй рекомендацию: `replace`-директива vs копирование пакетов. Сохрани отчёт в `docs/superpowers/notes/2026-06-21-legacy-mintrud-generator-audit.md`. Это research-задача, тестов нет. Ветка: `slice/r-documents-research`. НЕ трогай `go.mod` основного проекта в этой ветке — только отчёт."

**Важно:** эти три ветки конфликтуют только в `backend/app/router.go` (F1 его правит, F2 и R — нет). После завершения всех трёх ты делаешь merge в порядке F2 → F1 → R, разруливая конфликты.

### Шаг 4. Как ты координируешь

- **Веди общий task list** через `TaskCreate` / `TaskUpdate`. Один task на slice. Не используй task list для sub-task'ов — это забота sub-agent'ов.
- **Sub-agent'ы запускаются параллельно** через один `Agent`-tool-блок с тремя вызовами (как в system reminder твоего harness'а).
- **Параллельный лимит:** одновременно не больше 3-4 sub-agent'ов.
- **Сериализуемый ресурс:** `backend/app/router.go` и `backend/ui/layouts/shell.templ`. Любой sub-agent, который их правит, должен предупреждать тебя. Ты либо разрешаешь по очереди, либо сам делаешь финальную сборку роутов после merge их веток.
- **Контракты (секция 0.2 плана) — заморожены.** Если sub-agent предлагает изменить контракт — останови его, обсуди с пользователем.
- **После каждого slice:** прогоняй `make schema-test && make test && make sqlc && make templ`, убеждайся что всё зелёное, делаешь commit/merge, отмечаешь чекбокс в плане и соответствующий пункт в «Приёмке MVP».

### Шаг 5. Фазы после 0 (короткий гайд)

- **Фаза 1 (после F1+F2):** запусти F3 (auth). Это блокирующий slice для всех domain slices.
- **Фаза 2 (после F1+F2+F3):** запусти D1, D2, D4 параллельно. D3 стартует только после R (отчёт готов) и D2 (модель Protocol готова).
- **Фаза 3 (после D1+D2+D3):** E1 e2e integration tests. Это **критерий приёмки MVP**.
- **Фаза 4:** E2 runbook + verification + отметить все чекбоксы.

Полная детализация каждого slice — в плане. Не переизобретай.

### Шаг 6. Что НЕ делать

- **Не запускай Moodle** — не в скоупе.
- **Не делай MSI installer / Windows service deploy** — не в скоупе.
- **Не добавляй background queue** (Redis, RabbitMQ, asynq) — MVP in-process.
- **Не подключай PostgreSQL** — MVP на SQLite.
- **Не пропускай тесты.** Каждый slice имеет обязательные тесты — если sub-agent говорит «сделал, но тесты потом» — не принимай slice.
- **Не делай `--no-verify`** или обход хуков.
- **Не делегируй D3 другому агенту, пока R не закрыт** — без research отчёта D3 бесполезен.

### Шаг 7. Критерий твоего завершения

Ты завершаешь работу, когда:

1. `go test ./tests/e2e/... -run TestFullCycle` — зелёный.
2. `go test ./tests/e2e/... -run TestFullCycle_Restart` — зелёный.
3. `make schema-test && make test && make sqlc && make templ` — зелёные.
4. Все чекбоксы в плане `2026-06-21-mintrud-admin-core-mvp.md` отмечены.
5. `README.md` обновлён, `docs/runbook.md` создан.
6. Пользователь сделал manual smoke-проход e2e сценария через браузер (предложи ему это, когда пункты 1-5 готовы).

### Шаг 8. Коммуникация с пользователем

- Перед стартом Phase 0 — короткое подтверждение плана действий (3-5 предложений).
- После каждого slice — однострочный апдейт (что сделано, что следующее, есть ли блокеры).
- Если столкнулся с решением, не описанным в плане и не входящим в «Зафиксированные параметры» — **не выдумывай**, спроси пользователя.
- Если legacy `mintrud_generator` оказывается устроен сильно иначе, чем ожидалось (например, требует CGO, или только Windows-only) — останови D3 и обсуди с пользователем варианты.
- После полного завершения — финальный summary с критериями приёмки и известными post-MVP risks.

### Твой первый ответ пользователю

После чтения плана и перед запуском sub-agent'ов дай короткий ответ пользователю в формате:

```
Прочитал план Core MVP (10 slices, 4 фазы).
Зафиксированные параметры: [scope, auth, legacy, e2e — короткими тегами].
Запускаю Фазу 0 параллельно: F1 (slice completion), F2 (schema cleanup), R (legacy research).
Ожидаемый таймлайн до Phase 1: [оценка].
Сообщу после завершения всех трёх.
```

Затем стартуй три `Agent` tool calls в одном блоке.

**Начинай.**

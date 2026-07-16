# Crash-safe staging промышленного импорта

Дата: 2026-07-16
Статус: согласован для письменного ревью

## 1. Контекст и граница этапа

Upload API уже выполняет preflight промышленной XLSX-книги, сохраняет её во
временном приватном хранилище и создаёт durable FIFO-задание. Legacy parser
потоково выдаёт очищенные `SourceRow`, а схема SQLite уже содержит
`import_sheets`, `import_rows` и `legacy_import_rows`.

Следующий этап реализует только worker foundation и фазу staging. Он не
запускает goroutine вместе с HTTP-сервером и не изменяет канонические таблицы
`employers`, `workers`, `programs`, `training_records`, `protocols` и связанные
сущности. До готовности validating/applying pipeline задания, созданные через
API, остаются `queued`.

Такое разделение не позволяет незавершённому worker начать обрабатывать
пользовательские задания и оставлять их в промежуточном состоянии.

## 2. Цели

- Восстановить безопасный parser plan из сохранённых метаданных листов.
- Проверить неизменность временного файла до чтения бизнес-данных.
- Потоково сохранить очищенные строки без загрузки книги целиком в память.
- Фиксировать строки, sheet progress, import progress, heartbeat и lease одной
  транзакцией на пачку.
- Продолжить staging после crash или истечения lease без повторных строк.
- После полного staging crash-safe удалить исходный XLSX и очистить file token.
- Никогда не сохранить пароль, raw secret, полный путь или персональные данные
  в логах и ошибках задания.

## 3. Не входит в этап

- Нормализация ИНН, СНИЛС, ФИО, дат и остальных бизнес-полей.
- Сопоставление работодателей, сотрудников, программ и протоколов.
- Создание `import_row_issues`.
- Запись в канонические таблицы.
- Итоговые статусы `completed` и `completed_with_issues`.
- Retry/cancel/rows HTTP API.
- Фоновый polling loop и подключение worker к lifecycle сервера.
- Новая миграция или изменение `001_baseline.sql`.

## 4. Компоненты

### 4.1 Versioned parser plan codec

Пакет `backend/imports/legacy` получает codec сохранённого плана листа. Codec
хранит:

- версию формата;
- номер header row;
- logical field map;
- имена дополнительных колонок;
- индексы secret-колонок без названий и значений.

Сейчас enqueue сохраняет fields и extra fields, но не позиции password column.
После восстановления такой план не может отличить секретную колонку от
неизвестной и безопасно запустить parser. Новый codec устраняет этот разрыв.

Формат версии `v1` декодируется строго: неизвестная версия, некорректный индекс,
повторяющееся назначение или несовместимый profile дают безопасную permanent
ошибку. Существующие локальные queued-задания со старым versionless payload не
обрабатываются молча: они получают `staging_plan_incompatible` и могут быть
перезагружены после очистки developer DB. Программа ещё не находится в
production, поэтому backward migration этого промежуточного формата не нужна.

### 4.2 Stager

Новый `imports.Stager` принимает import, уже захваченный будущим worker через
`ClaimNextImport`. Он не выбирает задание самостоятельно и не создаёт goroutine.
Это сохраняет одну ответственность: превратить один immutable XLSX в durable
staging rows.

Конфигурация этапа:

- размер пачки по умолчанию: 250 строк;
- lease duration по умолчанию: 2 минуты;
- workbook limits: те же `legacy.Limits`, которые использовал preflight.

Stager требует `status=processing`, профиль `legacy_registry`, непустой
`lease_owner`, file token, ожидаемый SHA-256 и размер. Все state/progress SQL
updates проверяют текущий lease owner. Потерявший lease экземпляр не может
зафиксировать пачку или сменить phase.

### 4.3 SQL boundaries

SQL-запросы получают отдельные lease-scoped операции:

- начать или продолжить `staging`;
- атомарно обновить import heartbeat/progress;
- отметить полный staging и перейти в `validating`;
- очистить temp file token после удаления;
- получить import row по координате для диагностики нарушенного checkpoint;
- сохранить versioned header map при enqueue.

Административные операции будущих retry/cancel не переиспользуют worker query
без lease owner.

## 5. Поток обработки

1. Проверить входной import и конфигурацию.
2. Проверить `temp_file_expires_at`, затем разрешить путь через
   `FileStore.Path`; путь не логируется и не сохраняется в
   новых таблицах.
3. Потоково пересчитать размер и SHA-256 файла с проверкой context.
4. Сравнить их с durable metadata импорта. Несовпадение останавливает staging.
5. Загрузить `import_sheets`, строго декодировать каждый plan и проверить
   согласованность имени, order и profile.
6. Lease-scoped переходом установить phase `staging` и heartbeat.
7. Запустить `legacy.Parse` и принимать строки в порядке sheet/row.
8. Для каждого листа пропустить первые `rows_staged` emitted-строк. Это durable
   checkpoint: источник immutable и parser deterministic.
9. Новые строки собирать в пачки до 250 элементов.
10. Одной SQLite-транзакцией для пачки:
    - проверить lease через lease-scoped progress update;
    - создать `import_rows` с очищенным JSON;
    - создать связанные `legacy_import_rows` со source fingerprint;
    - обновить `rows_found`/`rows_staged` листа;
    - обновить `rows_total`, heartbeat и lease expiry импорта;
    - commit всей пачки.
11. При переходе к следующему листу committed checkpoint предыдущего листа
    получает status `staged`. После EOF все листы сверяются с parser stats.
12. Отдельной транзакцией выставить `staged_at`, phase `validating`, финальный
    `rows_total` и продлить lease. File token пока сохраняется.
13. Идемпотентно удалить временный XLSX через `FileStore.Delete`.
14. Lease-scoped запросом очистить `temp_file_token` и expiry.

`rows_processed`, `rows_applied`, `rows_duplicate` и `rows_needs_review` на этом
этапе не увеличиваются: они принадлежат validating/applying фазам.

## 6. Crash recovery и идемпотентность

`rows_staged` обновляется в той же транзакции, что обе staging-записи. Поэтому
возможны только два состояния пачки: вся пачка и checkpoint существуют либо не
существует ничего из них.

После restart новый lease owner снова читает immutable файл с начала, считает
emitted-строки каждого листа и пропускает уже подтверждённое количество.
Уникальный ключ `(import_id, sheet_name, row_number)` остаётся дополнительным
backstop. Если checkpoint и строки противоречат друг другу, stager возвращает
permanent `staging_checkpoint_corrupt`, не пытаясь исправить промышленные данные
автоматически.

Очистка файла выполняется в два durable шага:

1. БД уже содержит полный staging и `staged_at`.
2. Файл удаляется, затем token очищается.

Crash между шагами оставляет token, по которому повторный вызов выполняет
только cleanup. `Delete` отсутствующего файла считается успешным. Validation
не должна начинаться, пока token не очищен.

## 7. Ошибки и ownership

Stager возвращает типизированную безопасную ошибку с code, retryability и
внутренней причиной, доступной только вызывающему worker. На этом этапе stager
не выставляет terminal status: будущий orchestration layer решит, повторять
ошибку или переводить задание в `failed`.

Классы ошибок:

- context cancellation — остановить текущую работу, оставить последний
  committed checkpoint и дождаться истечения lease;
- потеря lease — немедленно прекратить работу, текущая транзакция откатывается;
- временная SQLite/filesystem ошибка — retryable;
- несовпадающий SHA/размер — permanent `source_file_mismatch`;
- отсутствующий/истёкший файл до полного staging — permanent
  `source_file_unavailable`;
- несовместимый plan — permanent `staging_plan_incompatible`;
- противоречивый checkpoint — permanent `staging_checkpoint_corrupt`;
- parser limit/structure failure после preflight — permanent `staging_failed`;
- ошибка удаления после полного staging — retryable `source_cleanup_failed`.

Ошибки не содержат filesystem path, token, SHA, filename, raw row или значения
ячеек. Stager не пишет PII в лог; будущий worker сможет логировать только import
ID, phase, sheet, row number, code и длительность.

## 8. Граничные случаи

- В книге есть пустые физические строки: checkpoint считает только emitted
  `SourceRow`, а не номер Excel-строки.
- Лист не содержит business rows: он завершается как `staged` с нулевыми
  счётчиками, если parser contract допускает такой план.
- Пачка пересекает границу листов: перед сменой листа текущая пачка flush-ится;
  одна пачка принадлежит ровно одному листу.
- Context отменён во время hash, parse или SQL: новая незакоммиченная работа не
  сохраняется.
- SQLite падает на второй строке пачки: первая строка этой пачки также
  откатывается.
- Lease сменился перед commit: lease-scoped update возвращает no rows и вся
  транзакция откатывается.
- Staging уже завершён, но token остался: выполняется только file cleanup.
- Файл исчез после полного staging: идемпотентный cleanup очищает token.
- Файл исчез до полного staging: данные не считаются полными и validation не
  начинается.

## 9. Файлы

Новые файлы:

- `backend/imports/stager.go` — orchestration одной staging-фазы;
- `backend/imports/stager_test.go` — интеграционные SQLite/filesystem тесты;
- `backend/imports/legacy/plan_codec.go` — versioned safe plan codec;
- `backend/imports/legacy/plan_codec_test.go` — round-trip и security tests.

Изменяемые файлы:

- `backend/imports/types.go` — staging config и typed outcome/error contracts;
- `backend/imports/service.go` — enqueue использует единый plan codec;
- `backend/imports/service_test.go` — проверка нового persisted plan;
- `backend/storage/queries/imports.sql` — lease-scoped phase/progress/cleanup;
- `backend/storage/queries/import_sheets.sql` — checkpoint/status operations;
- generated `backend/storage/db/imports.sql.go` и
  `backend/storage/db/import_sheets.sql.go`.

`backend/app`, `backend/internal/runner`, HTTP routes, frontend и migration в
этом этапе не меняются.

## 10. Тестирование

### Plan codec

- round-trip пяти профилей листов;
- secret column indexes восстанавливаются, но их header/value отсутствует в
  JSON;
- versionless, unknown version и повреждённый plan отклоняются;
- восстановленный plan продолжает удалять password column при parser run.

### Staging

- синтетическая пятилистовая книга полностью сохраняется;
- `raw_data` и staging tables не содержат password;
- workbook обрабатывается streaming parser-ом;
- batch size соблюдается, прогресс монотонен;
- sheet и import counters совпадают с фактическими строками;
- после success phase равна `validating`, `staged_at` заполнен, файл и token
  удалены;
- повтор после первого committed batch продолжает checkpoint без дублей;
- SQLite failure внутри пачки откатывает всю пачку;
- потеря lease откатывает пачку;
- SHA/size mismatch, отсутствующий файл и incompatible plan классифицируются
  безопасно;
- ошибка file cleanup повторяется без повторного parser run;
- cancellation сохраняет только ранее committed batches.

### Regression

- `go test -count=1 ./imports ./imports/legacy ./storage`;
- `go test -race -count=1 ./imports ./imports/legacy`;
- `go test -count=1 ./...`;
- `sh tests/run_schema_tests.sh`;
- повторный `sqlc generate` не меняет generated output;
- `git diff --check`.

Реальный файл из `temp` не копируется в fixtures, git или тестовые логи.

## 11. Критерии готовности этапа

- Один claimed import можно полностью и ограниченно по памяти превратить в
  durable staging rows.
- Все записи пачки и её checkpoint атомарны.
- Повтор после crash не создаёт дубли и продолжает с committed checkpoint.
- Потерявший lease stager не изменяет import.
- После полного staging исходный XLSX и durable token удалены crash-safe.
- Пароль отсутствует в persisted plan, raw data, staging rows, ошибках и логах.
- Phase готова к следующему этапу `validating`, но runtime worker ещё не
  запускается.

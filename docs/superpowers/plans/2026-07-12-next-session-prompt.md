# Next Session Prompt

Paste this into a fresh Codex session.

```text
Работаем в репозитории `/Users/ione/src/ikc_expert_admin_panel`.

Контекст:
- Нужно начать реализацию нового frontend shell для IKC Expert Mintrud Admin.
- Старый backend-rendered UI должен быть удален только на позднем этапе, после готовности frontend и auth boundary.
- Дизайн утвержден и сохранен в:
  `docs/superpowers/specs/2026-07-12-frontend-shell-design.md`
- Implementation plan сохранен в:
  `docs/superpowers/plans/2026-07-12-frontend-shell-implementation.md`
- План нужно исполнять строго stage-by-stage.

Обязательные правила исполнения:
1. Сначала прочитай `AGENTS.md`, design spec и implementation plan.
2. Используй нужные skills по правилам среды. Для реализации плана используй `superpowers:subagent-driven-development` или `superpowers:executing-plans`.
3. Начни со Stage 1: Frontend Foundation.
4. Не делай commit после отдельных tasks.
5. После завершения stage:
   - покажи, что изменилось;
   - дай локальный URL или скриншоты, если есть видимый UI;
   - покажи точные verification commands и результаты;
   - покажи `git status --short`;
   - спроси: `Approve Stage N commit?`
6. Commit делай только после моего явного approval.
7. Если я прошу правки, внеси их, rerun verification, снова покажи stage, и снова спроси approval.
8. Не трогай `.env`, не печатай значения секретов.
9. Docker compose URL для проверки: `http://localhost:8081/login`, не `8080`.
10. Не удаляй backend `templ` UI до Stage 5.

Начни с краткого подтверждения Stage 1 goal, затем приступай к Task 1 из плана.
```

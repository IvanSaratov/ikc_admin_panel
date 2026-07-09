package app

import (
	"net/http"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/go-chi/chi/v5"
)

func registerRoutes(router chi.Router, deps Deps, c *container) {
	// Публичные маршруты входа. RequireAuth здесь намеренно не применяется,
	// чтобы неавторизованные пользователи могли открыть форму входа.
	router.With(requestLogger(deps.Log)).Get("/login", c.adminHandler.GetLogin)
	if deps.LoginRate != nil {
		router.With(requestLogger(deps.Log), admin.LoginRateLimitMiddleware(deps.LoginRate, deps.Log, c.auditSvc)).
			Post("/login", c.adminHandler.PostLogin)
	} else {
		router.With(requestLogger(deps.Log)).Post("/login", c.adminHandler.PostLogin)
	}

	// Защищенная группа: все остальные маршруты проходят авторизацию, actor и лог запроса.
	router.Group(func(r chi.Router) {
		r.Use(c.requireAuth)
		r.Use(withActor)
		r.Use(requestLogger(deps.Log))

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/programs", http.StatusSeeOther)
		})
		// Выход принимает только POST с CSRF, чтобы внешняя ссылка или
		// предзагрузка не могли завершить сессию.
		r.Post("/logout", c.adminHandler.PostLogout)

		// Журнал аудита: только просмотр, без записи в action_log.
		r.Get("/audit", c.auditHandler.List)

		// Программы: группы.
		r.Get("/programs", c.programHandler.List)
		r.Post("/programs/groups", c.programHandler.CreateGroup)
		r.Get("/programs/groups/{id}/edit", c.programHandler.EditGroup)
		r.Post("/programs/groups/{id}/edit", c.programHandler.EditGroup)
		r.Post("/programs/groups/{id}/deactivate", c.programHandler.DeactivateGroup)

		// Программы: программы.
		r.Post("/programs", c.programHandler.CreateProgram)
		r.Get("/programs/{id}/edit", c.programHandler.EditProgram)
		r.Post("/programs/{id}/edit", c.programHandler.EditProgram)
		r.Post("/programs/{id}/deactivate", c.programHandler.DeactivateProgram)

		// Работодатели.
		r.Get("/employers", c.employerHandler.List)
		r.Post("/employers", c.employerHandler.Create)
		r.Get("/employers/{id}", c.employerHandler.Detail)
		r.Get("/employers/{id}/edit", c.employerHandler.Edit)
		r.Post("/employers/{id}", c.employerHandler.Edit)
		r.Post("/employers/{id}/deactivate", c.employerHandler.Deactivate)

		// Работники и назначения.
		r.Get("/workers", c.peopleHandler.List)
		r.Post("/workers", c.peopleHandler.CreateWorker)
		r.Get("/workers/{id}", c.peopleHandler.Detail)
		r.Get("/workers/{id}/edit", c.peopleHandler.Edit)
		r.Post("/workers/{id}", c.peopleHandler.Edit)
		r.Post("/workers/assignments", c.peopleHandler.AssignEmployer)
		r.Post("/workers/assignments/{id}/deactivate", c.peopleHandler.DeactivateAssignment)

		// Протоколы: список, создание, детали, исправление, переходы, участники.
		r.Get("/protocols", c.protocolHandler.List)
		r.Post("/protocols", c.protocolHandler.Create)
		r.Get("/protocols/{id}", c.protocolHandler.Detail)
		r.Get("/protocols/{id}/fix", c.protocolHandler.Fix)
		r.Post("/protocols/{id}/fix", c.protocolHandler.Fix)
		r.Post("/protocols/{id}/participants", c.protocolHandler.AddParticipant)
		r.Post("/protocols/{id}/participants/{pid}", c.protocolHandler.RemoveParticipant)
		r.Post("/protocols/{id}/transition", c.protocolHandler.Transition)

		// Документы: генерация и скачивание.
		r.Post("/protocols/{id}/generate", c.documentHandler.Generate)
		r.Get("/protocols/{id}/download", c.documentHandler.Download)

		// Заявки: загрузка XLSX и подготовленные строки.
		r.Get("/requests", c.requestHandler.List)
		r.Get("/requests/new", c.requestHandler.NewRequestForm)
		r.Post("/requests/new", c.requestHandler.Upload)
		r.Get("/requests/{id}", c.requestHandler.Detail)
		r.Post("/requests/{id}/rows/{rowID}/apply", c.requestHandler.ApplyRow)
		r.Post("/requests/{id}/rows/{rowID}/skip", c.requestHandler.SkipRow)
	})
}

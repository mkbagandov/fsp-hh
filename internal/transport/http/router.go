// Package httptransport — HTTP-слой: роутер chi, OpenAPI через huma,
// middleware и регистрация операций по модулям.
package httptransport

import (
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/infrastructure/authtoken"
	"fsp/internal/infrastructure/mail"
)

// Deps — зависимости HTTP-слоя. Бизнес-логика живёт в internal/app,
// сюда передаются уже собранные сервисы.
type Deps struct {
	Logger  *slog.Logger
	Pool    *pgxpool.Pool
	Version string
	IsDev   bool
	// CORSOrigins — разрешённые источники (dev-сервер фронтенда); пусто — CORS выключен.
	CORSOrigins []string
	// RateLimitPerMinute — общий лимит запросов на IP; 0 — без лимита (тесты).
	RateLimitPerMinute int
	// Tokens — проверка access-токенов (nil — аутентификация выключена, например при экспорте OpenAPI).
	Tokens *authtoken.Issuer
	// Auth — модуль аутентификации (nil — операции не регистрируются).
	Auth *AuthDeps
	// Outbox — письма консольного мейлера для /dev/outbox (только dev).
	Outbox mail.Outbox
	// FSP — вход и привязка через ФСП ID (nil — операции не регистрируются).
	FSP *FSPAuthDeps
	// Candidate — профиль кандидата (nil — операции не регистрируются).
	Candidate *CandidateDeps
	// Testing — опрос и адаптивный тест (nil — операции не регистрируются).
	Testing *TestingDeps
	// Employer — компания, потребности, подбор и приглашения (nil — операции не регистрируются).
	Employer *EmployerDeps
	// Vacancies — вакансии, отклики и уведомления (nil — операции не регистрируются).
	Vacancies *VacancyDeps
	// Admin — администрирование и /metrics (nil — операции не регистрируются).
	Admin *AdminDeps
	// WebDir — каталог собранного фронтенда; пусто — SPA не раздаётся (только API и /docs).
	WebDir string
	// TrustedProxies — прокси, которым доверяется X-Forwarded-For (пусто — заголовок игнорируется).
	TrustedProxies []netip.Prefix
	// HSTS — отправлять Strict-Transport-Security (за HTTPS).
	HSTS bool
	// MetricsToken — токен для /metrics; вне dev без токена /metrics не публикуется.
	MetricsToken string
	// UserActive — проверка статуса пользователя для access-токенов (nil — без проверки).
	UserActive UserActiveFunc
}

// APIPrefix — префикс всех операций публичного API.
const APIPrefix = "/api/v1"

// NewRouter собирает полный HTTP-обработчик приложения и возвращает huma.API
// для экспорта OpenAPI без запуска сервера.
func NewRouter(d Deps) (http.Handler, huma.API) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// middleware.RealIP намеренно не используется: он доверяет X-Forwarded-For от кого угодно.
	// realIP берёт адрес клиента из заголовка только за прокси из TRUSTED_PROXIES.
	r.Use(realIP(d.TrustedProxies))
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders(d.HSTS))
	r.Use(middleware.Compress(5))
	r.Use(corsMiddleware(d.CORSOrigins))
	if d.RateLimitPerMinute > 0 {
		r.Use(rateLimit(d.RateLimitPerMinute))
		// Строгий лимит на маршруты с паролями и кодами; обновление сессии — отдельный, мягкий лимит.
		r.Use(rateLimitPaths(credentialPaths, 30))
		r.Use(rateLimitPaths([]string{"/auth/refresh"}, 120))
	}
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(requestMeta)
	status := newStatusCache(d.UserActive)
	r.Use(authenticate(d.Tokens, status))
	r.Use(uploadGuard)

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) { writeProblem(w, http.StatusNotFound, "Not Found") })
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	})

	// Технический health для балансировщиков и docker healthcheck: без БД, без JSON.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	if d.Admin != nil && d.Admin.Service != nil && (d.IsDev || d.MetricsToken != "") {
		r.Get("/metrics", metricsGuard(d.MetricsToken, metricsHandler(d.Admin.Service)).ServeHTTP)
	}
	r.Get("/docs", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, APIPrefix+"/docs", http.StatusTemporaryRedirect)
	})

	var api huma.API
	r.Route(APIPrefix, func(r chi.Router) {
		cfg := huma.DefaultConfig("ФСП — платформа найма ИТ-специалистов", d.Version)
		cfg.Info.Description = "API платформы подбора ИТ-специалистов с обратной механикой: " +
			"кандидаты распределяются по категориям «специализация × грейд» по результатам " +
			"опроса и тестирования, работодатель находит категорию и приглашает конкретного кандидата. " +
			"Ошибки возвращаются в формате application/problem+json (RFC 9457); поле `errors[].value` " +
			"содержит машиночитаемый код ошибки. Аутентификация: заголовок `Authorization: Bearer <access_token>`, " +
			"refresh-токен — в HttpOnly cookie `fsp_refresh`."
		cfg.DocsPath = "/docs"
		cfg.OpenAPIPath = "/openapi"
		cfg.Servers = []*huma.Server{{URL: APIPrefix, Description: "Текущий сервер"}}
		cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
			"bearer": {Type: "http", Scheme: "bearer", BearerFormat: "JWT", Description: "Access-токен из /auth/login, /auth/verify или /auth/refresh"},
		}
		api = humachi.New(r, cfg)

		registerSystem(api, d)
		registerRefdata(api)
		if d.Auth != nil {
			registerAuth(api, *d.Auth, status)
		}
		if d.FSP != nil {
			registerFSPAuth(api, *d.FSP)
		}
		if d.Candidate != nil {
			registerCandidate(api, *d.Candidate)
			registerCandidateResume(api, *d.Candidate)
			registerPublicVerify(api, *d.Candidate)
		}
		if d.Testing != nil {
			registerTesting(api, *d.Testing)
		}
		if d.Employer != nil {
			registerEmployer(api, *d.Employer)
		}
		if d.Vacancies != nil {
			registerVacancies(api, *d.Vacancies)
		}
		if d.Admin != nil {
			registerAdmin(api, *d.Admin, status)
		}
		if d.IsDev && d.Outbox != nil {
			registerDev(api, d.Outbox)
		}
	})

	if d.WebDir != "" {
		// Фронтенд ниже API-маршрутов: chi выбирает более конкретный маршрут, поэтому /api/v1/* и /healthz не перекрываются.
		r.Handle("/*", spaHandler(d.WebDir))
	}

	return r, api
}

// requestLogger пишет одну структурированную строку на запрос.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"dur_ms", time.Since(start).Milliseconds(),
				"req_id", middleware.GetReqID(r.Context()),
				"ip", r.RemoteAddr,
			)
		})
	}
}

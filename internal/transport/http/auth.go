package httptransport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"fsp/internal/app/auth"
	"fsp/internal/domain"
)

const refreshCookie = "fsp_refresh"

// AuthDeps — зависимости модуля аутентификации.
type AuthDeps struct {
	Service      *auth.Service
	CookieSecure bool // Secure-флаг cookie (false только локально по http)
	RefreshTTL   time.Duration
}

type sessionBody struct {
	AccessToken  string        `json:"access_token" doc:"JWT для заголовка Authorization: Bearer"`
	ExpiresAt    time.Time     `json:"expires_at" doc:"Срок действия access-токена (UTC)"`
	User         auth.UserInfo `json:"user"`
	RefreshInSet bool          `json:"refresh_cookie_set" doc:"Refresh-токен установлен в HttpOnly cookie fsp_refresh"`
}

// SessionOutput — ответ со входом: тело + cookie с refresh-токеном.
type SessionOutput struct {
	SetCookie []http.Cookie `header:"Set-Cookie"`
	Body      sessionBody
}

type registerInput struct {
	Body struct {
		Email    string `json:"email" format:"email" maxLength:"254" doc:"E-mail, на него придёт код подтверждения"`
		Password string `json:"password" minLength:"8" maxLength:"128"`
		Role     string `json:"role" enum:"candidate,employer" doc:"Роль: кандидат или работодатель"`
		Consents struct {
			Processing bool `json:"processing" doc:"Согласие на обработку персональных данных (обязательно)"`
		} `json:"consents"`
	}
}

type registerOutput struct {
	Status int
	Body   struct {
		User                 auth.UserInfo `json:"user"`
		VerificationRequired bool          `json:"verification_required"`
		Message              string        `json:"message"`
	}
}

func registerAuth(api huma.API, d AuthDeps, status *statusCache) {
	meta := func(ctx context.Context) auth.RequestMeta {
		return auth.RequestMeta{IP: clientIPFromCtx(ctx), UserAgent: userAgentFromCtx(ctx)}
	}
	withSession := func(s auth.Session) *SessionOutput {
		out := &SessionOutput{}
		out.Body = sessionBody{AccessToken: s.AccessToken, ExpiresAt: s.AccessExpiry, User: s.User, RefreshInSet: true}
		out.SetCookie = []http.Cookie{refreshCookieFor(s.RefreshToken, s.RefreshExp, d.CookieSecure)}
		return out
	}

	huma.Register(api, huma.Operation{
		OperationID: "auth-register", Method: http.MethodPost, Path: "/auth/register",
		Summary: "Регистрация по e-mail", Tags: []string{"auth"}, DefaultStatus: http.StatusCreated,
		Description: "Создаёт учётную запись кандидата или работодателя и отправляет 6-значный код на e-mail. " +
			"Требуется согласие на обработку персональных данных. Ошибки: 409 email_taken, 422 consent_required.",
	}, func(ctx context.Context, in *registerInput) (*registerOutput, error) {
		u, err := d.Service.Register(ctx, auth.RegisterInput{
			Email: in.Body.Email, Password: in.Body.Password, Role: domain.Role(in.Body.Role),
			ProcessingConsent: in.Body.Consents.Processing,
		}, meta(ctx))
		if err != nil {
			return nil, MapError(err)
		}
		out := &registerOutput{Status: http.StatusCreated}
		out.Body.User = u
		out.Body.VerificationRequired = true
		out.Body.Message = "Код подтверждения отправлен на " + u.Email
		return out, nil
	})

	type verifyInput struct {
		Body struct {
			Email string `json:"email" format:"email"`
			Code  string `json:"code" minLength:"6" maxLength:"6" pattern:"^[0-9]{6}$"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-verify-email", Method: http.MethodPost, Path: "/auth/verify",
		Summary: "Подтверждение e-mail кодом", Tags: []string{"auth"},
		Description: "Проверяет код и открывает сессию. Ошибки: 422 code_invalid, 429 code_attempts_exceeded.",
	}, func(ctx context.Context, in *verifyInput) (*SessionOutput, error) {
		s, err := d.Service.VerifyEmail(ctx, in.Body.Email, in.Body.Code, meta(ctx))
		if err != nil {
			return nil, MapError(err)
		}
		return withSession(s), nil
	})

	type emailInput struct {
		Body struct {
			Email string `json:"email" format:"email"`
		}
	}
	type acceptedOutput struct {
		Status int
		Body   struct {
			Message string `json:"message"`
		}
	}
	accepted := func(msg string) *acceptedOutput {
		o := &acceptedOutput{Status: http.StatusAccepted}
		o.Body.Message = msg
		return o
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-verify-resend", Method: http.MethodPost, Path: "/auth/verify/resend",
		Summary: "Повторная отправка кода", Tags: []string{"auth"}, DefaultStatus: http.StatusAccepted,
		Description: "Ответ одинаков для любого e-mail. Ошибка 429 resend_limit при превышении лимита.",
	}, func(ctx context.Context, in *emailInput) (*acceptedOutput, error) {
		if err := d.Service.ResendVerification(ctx, in.Body.Email); err != nil {
			return nil, MapError(err)
		}
		return accepted("Если адрес зарегистрирован и не подтверждён, код отправлен повторно"), nil
	})

	type loginInput struct {
		Body struct {
			Email    string `json:"email" format:"email"`
			Password string `json:"password" minLength:"1" maxLength:"128"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-login", Method: http.MethodPost, Path: "/auth/login",
		Summary: "Вход по e-mail и паролю", Tags: []string{"auth"},
		Description: "Ошибки: 401 invalid_credentials, 403 email_not_verified, 403 user_blocked.",
	}, func(ctx context.Context, in *loginInput) (*SessionOutput, error) {
		s, err := d.Service.Login(ctx, in.Body.Email, in.Body.Password, meta(ctx))
		if err != nil {
			return nil, MapError(err)
		}
		return withSession(s), nil
	})

	type refreshInput struct {
		Cookie string `cookie:"fsp_refresh"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-refresh", Method: http.MethodPost, Path: "/auth/refresh",
		Summary: "Обновление сессии по refresh-cookie", Tags: []string{"auth"},
		Description: "Ротирует refresh-токен (старый отзывается). Повторное использование отозванного токена " +
			"закрывает все сессии пользователя. Ошибка 401 refresh_invalid.",
	}, func(ctx context.Context, in *refreshInput) (*SessionOutput, error) {
		s, err := d.Service.Refresh(ctx, in.Cookie, meta(ctx))
		if err != nil {
			return nil, MapError(err)
		}
		return withSession(s), nil
	})

	type logoutOutput struct {
		SetCookie []http.Cookie `header:"Set-Cookie"`
		Body      struct {
			Message string `json:"message"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-logout", Method: http.MethodPost, Path: "/auth/logout",
		Summary: "Выход (отзыв refresh-токена)", Tags: []string{"auth"},
	}, func(ctx context.Context, in *refreshInput) (*logoutOutput, error) {
		_ = d.Service.Logout(ctx, in.Cookie)
		out := &logoutOutput{SetCookie: []http.Cookie{refreshCookieFor("", time.Unix(0, 0), d.CookieSecure)}}
		out.Body.Message = "Сессия завершена"
		return out, nil
	})

	type meOutput struct {
		Body struct {
			User     auth.UserInfo      `json:"user"`
			Consents []auth.ConsentInfo `json:"consents"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-me", Method: http.MethodGet, Path: "/auth/me",
		Summary: "Текущий пользователь и согласия", Tags: []string{"auth"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*meOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		u, consents, err := d.Service.Me(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		out := &meOutput{}
		out.Body.User, out.Body.Consents = u, consents
		return out, nil
	})

	type changePasswordInput struct {
		Body struct {
			CurrentPassword string `json:"current_password" maxLength:"128"`
			NewPassword     string `json:"new_password" minLength:"8" maxLength:"128"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-password-change", Method: http.MethodPost, Path: "/auth/password/change",
		Summary: "Смена пароля", Tags: []string{"auth"}, Security: bearerSecurity,
		Description: "Закрывает все остальные сессии. Ошибка 422 password_wrong.",
	}, func(ctx context.Context, in *changePasswordInput) (*acceptedOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Service.ChangePassword(ctx, p.UserID, in.Body.CurrentPassword, in.Body.NewPassword, meta(ctx)); err != nil {
			return nil, MapError(err)
		}
		return accepted("Пароль изменён, другие сессии закрыты"), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-password-forgot", Method: http.MethodPost, Path: "/auth/password/forgot",
		Summary: "Запрос кода для сброса пароля", Tags: []string{"auth"}, DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, in *emailInput) (*acceptedOutput, error) {
		if err := d.Service.ForgotPassword(ctx, in.Body.Email); err != nil {
			return nil, MapError(err)
		}
		return accepted("Если адрес зарегистрирован, код для сброса отправлен"), nil
	})

	type resetInput struct {
		Body struct {
			Email       string `json:"email" format:"email"`
			Code        string `json:"code" pattern:"^[0-9]{6}$"`
			NewPassword string `json:"new_password" minLength:"8" maxLength:"128"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-password-reset", Method: http.MethodPost, Path: "/auth/password/reset",
		Summary: "Сброс пароля по коду", Tags: []string{"auth"},
		Description: "Устанавливает новый пароль, подтверждает e-mail и закрывает все сессии. Ошибка 422 code_invalid.",
	}, func(ctx context.Context, in *resetInput) (*acceptedOutput, error) {
		if err := d.Service.ResetPassword(ctx, in.Body.Email, in.Body.Code, in.Body.NewPassword, meta(ctx)); err != nil {
			return nil, MapError(err)
		}
		return accepted("Пароль обновлён, войдите заново"), nil
	})

	type consentInput struct {
		Body struct {
			Kind string `json:"kind" enum:"processing,publication,fsp_data"`
		}
	}
	type consentOutput struct {
		Body auth.ConsentInfo
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-consent-give", Method: http.MethodPost, Path: "/auth/consents",
		Summary: "Выдать согласие", Tags: []string{"auth"}, Security: bearerSecurity,
		Description: "publication — на показ профиля работодателям; fsp_data — на получение достижений из реестра ФСП.",
	}, func(ctx context.Context, in *consentInput) (*consentOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		c, err := d.Service.GiveConsent(ctx, p.UserID, domain.ConsentKind(in.Body.Kind), meta(ctx))
		if err != nil {
			return nil, MapError(err)
		}
		return &consentOutput{Body: c}, nil
	})

	type consentPath struct {
		Kind string `path:"kind" enum:"publication,fsp_data"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-consent-withdraw", Method: http.MethodDelete, Path: "/auth/consents/{kind}",
		Summary: "Отозвать согласие", Tags: []string{"auth"}, Security: bearerSecurity,
		Description: "Согласие на обработку отозвать нельзя — используйте удаление аккаунта (409 consent_processing_required).",
	}, func(ctx context.Context, in *consentPath) (*acceptedOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Service.WithdrawConsent(ctx, p.UserID, domain.ConsentKind(in.Kind), meta(ctx)); err != nil {
			return nil, MapError(err)
		}
		return accepted("Согласие отозвано"), nil
	})

	type exportOutput struct {
		Body map[string]any
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-account-export", Method: http.MethodGet, Path: "/auth/account/export",
		Summary: "Выгрузка данных учётной записи (152-ФЗ)", Tags: []string{"auth"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*exportOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		data, err := d.Service.ExportAccount(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		return &exportOutput{Body: data}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-account-delete", Method: http.MethodDelete, Path: "/auth/account",
		Summary: "Удаление учётной записи", Tags: []string{"auth"}, Security: bearerSecurity,
		Description: "Учётная запись помечается удалённой, e-mail обезличивается, персональные данные кандидата " +
			"(контакты, резюме, опыт, образование, навыки, данные ФСП) стираются, доступы работодателей к контактам " +
			"отзываются, все сессии закрываются — в том числе уже выданный access-токен.",
	}, func(ctx context.Context, _ *struct{}) (*logoutOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Service.DeleteAccount(ctx, p.UserID, meta(ctx)); err != nil {
			return nil, MapError(err)
		}
		status.forget(p.UserID)
		out := &logoutOutput{SetCookie: []http.Cookie{refreshCookieFor("", time.Unix(0, 0), d.CookieSecure)}}
		out.Body.Message = "Учётная запись удалена"
		return out, nil
	})
}

// bearerSecurity — требование схемы bearer в OpenAPI (сама проверка — в обработчике).
var bearerSecurity = []map[string][]string{{"bearer": {}}}

func refreshCookieFor(value string, exp time.Time, secure bool) http.Cookie {
	// Secure берётся из конфигурации: локально API работает по http, на стенде за HTTPS — COOKIE_SECURE=true.
	c := http.Cookie{ //nolint:gosec // Secure управляется COOKIE_SECURE, HttpOnly и SameSite заданы
		Name:     refreshCookie,
		Value:    value,
		Path:     APIPrefix + "/auth",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  exp,
	}
	if value == "" {
		c.MaxAge = -1
	}
	return c
}

package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"fsp/internal/app/auth"
	"fsp/internal/app/fsp"
	"fsp/internal/domain"
	"fsp/internal/infrastructure/authtoken"
	"fsp/internal/infrastructure/oidc"
	"fsp/internal/infrastructure/secrets"
)

const oidcStateCookie = "fsp_oidc"

// FSPAuthDeps — вход и привязка через ФСП ID.
type FSPAuthDeps struct {
	Client       *oidc.Client
	Tokens       *authtoken.Issuer
	Service      *auth.Service
	Achievements *fsp.Syncer
	FrontendURL  string // куда возвращать браузер после callback
	CookieSecure bool
}

func registerFSPAuth(api huma.API, d FSPAuthDeps) {
	meta := func(ctx context.Context) auth.RequestMeta {
		return auth.RequestMeta{IP: clientIPFromCtx(ctx), UserAgent: userAgentFromCtx(ctx)}
	}

	type fspStartInput struct {
		Body struct {
			Mode              string `json:"mode" enum:"login,link" doc:"login — войти или зарегистрироваться через ФСП ID; link — привязать к текущему аккаунту (нужен bearer)"`
			ConsentProcessing bool   `json:"consent_processing" doc:"Согласие на обработку ПДн (обязательно, если аккаунт будет создан)"`
			ReturnTo          string `json:"return_to,omitempty" doc:"Относительный путь фронтенда после завершения, по умолчанию /auth/fsp/done"`
		}
	}
	type fspStartOutput struct {
		SetCookie []http.Cookie `header:"Set-Cookie"`
		Body      struct {
			AuthorizationURL string `json:"authorization_url" doc:"Адрес ФСП ID, на который нужно перенаправить браузер"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-fsp-start", Method: http.MethodPost, Path: "/auth/fsp/start",
		Summary: "Начать вход или привязку через ФСП ID", Tags: []string{"auth", "fsp"},
		Description: "Возвращает адрес авторизации (OpenID Connect, Authorization Code + PKCE) и ставит cookie с подписанным состоянием. " +
			"Браузер перенаправляется на ФСП ID, после входа ФСП ID возвращает на /auth/fsp/callback. Ошибка 503 fsp_id_disabled, если провайдер не настроен.",
	}, func(ctx context.Context, in *fspStartInput) (*fspStartOutput, error) {
		if d.Client == nil || !d.Client.Enabled() {
			return nil, huma.NewError(http.StatusServiceUnavailable, "Вход через ФСП ID не настроен", &huma.ErrorDetail{Value: "fsp_id_disabled"})
		}
		data := map[string]string{"mode": in.Body.Mode, "rt": safeReturnTo(in.Body.ReturnTo)}
		if in.Body.ConsentProcessing {
			data["consent"] = "1"
		}
		if in.Body.Mode == "link" {
			p, err := domain.RequirePrincipal(ctx)
			if err != nil {
				return nil, MapError(err)
			}
			data["uid"] = p.UserID.String()
		}
		state, _ := secrets.RandomToken(16)
		nonce, _ := secrets.RandomToken(16)
		verifier, _ := secrets.RandomToken(32)
		data["state"], data["nonce"], data["verifier"] = state, nonce, verifier
		authURL, err := d.Client.AuthURL(ctx, state, nonce, verifier)
		if err != nil {
			slog.Default().Warn("fsp id start failed", "err", err)
			return nil, huma.NewError(http.StatusBadGateway, "ФСП ID временно недоступен, попробуйте позже", &huma.ErrorDetail{Value: "fsp_id_unavailable"})
		}
		signed, err := d.Tokens.IssueState(data, 10*time.Minute)
		if err != nil {
			return nil, MapError(err)
		}
		out := &fspStartOutput{SetCookie: []http.Cookie{oidcCookie(signed, 600, d.CookieSecure)}}
		out.Body.AuthorizationURL = authURL
		return out, nil
	})

	type callbackInput struct {
		Code      string `query:"code"`
		State     string `query:"state"`
		Error     string `query:"error"`
		ErrorDesc string `query:"error_description"`
		Cookie    string `cookie:"fsp_oidc"`
	}
	type redirectOutput struct {
		Status    int
		Location  string        `header:"Location"`
		SetCookie []http.Cookie `header:"Set-Cookie"`
	}
	fail := func(returnTo, code string) *redirectOutput {
		return &redirectOutput{
			Status:    http.StatusFound,
			Location:  d.FrontendURL + returnTo + "?fsp_error=" + url.QueryEscape(code),
			SetCookie: []http.Cookie{oidcCookie("", -1, d.CookieSecure)},
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-fsp-callback", Method: http.MethodGet, Path: "/auth/fsp/callback",
		Summary: "Возврат из ФСП ID", Tags: []string{"auth", "fsp"}, DefaultStatus: http.StatusFound,
		Description: "Проверяет state и nonce, обменивает код на токены, входит/регистрирует или привязывает ФСП ID, " +
			"синхронизирует достижения, ставит refresh-cookie и перенаправляет на фронтенд (`return_to`, при ошибке — с `?fsp_error=`).",
	}, func(ctx context.Context, in *callbackInput) (*redirectOutput, error) {
		returnTo := "/auth/fsp/done"
		data, err := d.Tokens.ParseState(in.Cookie)
		if err != nil {
			return fail(returnTo, "state_invalid"), nil
		}
		returnTo = safeReturnTo(data["rt"])
		if in.Error != "" {
			return fail(returnTo, in.Error), nil
		}
		if in.State == "" || in.State != data["state"] {
			return fail(returnTo, "state_mismatch"), nil
		}
		id, err := d.Client.Exchange(ctx, in.Code, data["verifier"], data["nonce"])
		if err != nil {
			return fail(returnTo, "exchange_failed"), nil
		}
		var sess auth.Session
		switch data["mode"] {
		case "link":
			uid, err := uuid.Parse(data["uid"])
			if err != nil {
				return fail(returnTo, "state_invalid"), nil
			}
			if _, err := d.Service.LinkFSP(ctx, uid, id, meta(ctx)); err != nil {
				return fail(returnTo, errorCode(err)), nil
			}
			return &redirectOutput{Status: http.StatusFound, Location: d.FrontendURL + returnTo + "?fsp_linked=1",
				SetCookie: []http.Cookie{oidcCookie("", -1, d.CookieSecure)}}, nil
		default:
			sess, err = d.Service.CompleteFSPLogin(ctx, id, data["consent"] == "1", meta(ctx))
			if err != nil {
				return fail(returnTo, errorCode(err)), nil
			}
		}
		return &redirectOutput{
			Status:   http.StatusFound,
			Location: d.FrontendURL + returnTo + "?fsp_login=1",
			SetCookie: []http.Cookie{
				refreshCookieFor(sess.RefreshToken, sess.RefreshExp, d.CookieSecure),
				oidcCookie("", -1, d.CookieSecure),
			},
		}, nil
	})

	type linkOutput struct {
		Body auth.FSPLinkInfo
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-fsp-link-get", Method: http.MethodGet, Path: "/auth/fsp/link",
		Summary: "Состояние привязки ФСП ID", Tags: []string{"auth", "fsp"}, Security: bearerSecurity,
	}, func(ctx context.Context, _ *struct{}) (*linkOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		info, err := d.Service.FSPLink(ctx, p.UserID)
		if err != nil {
			return nil, MapError(err)
		}
		return &linkOutput{Body: info}, nil
	})

	type messageOutput struct {
		Status int
		Body   struct {
			Message string `json:"message"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "auth-fsp-unlink", Method: http.MethodDelete, Path: "/auth/fsp/link",
		Summary: "Отвязать ФСП ID", Tags: []string{"auth", "fsp"}, Security: bearerSecurity, DefaultStatus: http.StatusAccepted,
		Description: "Удаляет привязку и кэш достижений, отзывает согласие fsp_data.",
	}, func(ctx context.Context, _ *struct{}) (*messageOutput, error) {
		p, err := domain.RequirePrincipal(ctx)
		if err != nil {
			return nil, MapError(err)
		}
		if err := d.Service.UnlinkFSP(ctx, p.UserID, meta(ctx)); err != nil {
			return nil, MapError(err)
		}
		out := &messageOutput{Status: http.StatusAccepted}
		out.Body.Message = "ФСП ID отвязан, достижения удалены"
		return out, nil
	})

	type achievementsOutput struct {
		Body struct {
			Items []fsp.Achievement `json:"items"`
			Total int               `json:"total"`
		}
	}
	huma.Register(api, huma.Operation{
		OperationID: "candidate-fsp-achievements", Method: http.MethodGet, Path: "/candidate/fsp/achievements",
		Summary: "Достижения ФСП текущего кандидата", Tags: []string{"candidate", "fsp"}, Security: bearerSecurity,
		Description: "Кэш достижений из реестра ФСП, синхронизированный при входе или привязке. Пустой список — «нет истории ФСП».",
	}, func(ctx context.Context, _ *struct{}) (*achievementsOutput, error) {
		p, err := domain.RequireRole(ctx, domain.RoleCandidate)
		if err != nil {
			return nil, MapError(err)
		}
		out := &achievementsOutput{}
		out.Body.Items = []fsp.Achievement{}
		if d.Achievements != nil {
			items, err := d.Achievements.List(ctx, p.UserID)
			if err != nil {
				return nil, MapError(err)
			}
			out.Body.Items = items
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

func oidcCookie(value string, maxAge int, secure bool) http.Cookie {
	return http.Cookie{ //nolint:gosec // Secure управляется COOKIE_SECURE
		Name: oidcStateCookie, Value: value, Path: APIPrefix + "/auth/fsp",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	}
}

// safeReturnTo допускает только относительные пути фронтенда (защита от open redirect).
func safeReturnTo(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\\r\n") {
		return "/auth/fsp/done"
	}
	return p
}

func errorCode(err error) string {
	var de *domain.Error
	if ok := asDomainError(err, &de); ok {
		return de.Code
	}
	return "internal_error"
}

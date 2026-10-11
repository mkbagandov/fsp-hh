// Package oidc — клиент OpenID Connect для входа и привязки через ФСП ID.
// Провайдер задаётся issuer-адресом; в хакатоне это cmd/fspmock, в будущем — Keycloak ФСП.
package oidc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config — параметры клиента.
type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string // по умолчанию openid profile email fsp.achievements
}

// Identity — проверенные данные пользователя из id_token.
type Identity struct {
	Issuer        string
	Subject       string
	FSPID         string
	Email         string
	EmailVerified bool
	Name          string
	GivenName     string
	FamilyName    string
	AccessToken   string
	Expiry        time.Time
}

// Client — ленивый OIDC-клиент: discovery выполняется при первом обращении,
// чтобы API стартовал даже если провайдер поднимется позже.
type Client struct {
	cfg      Config
	mu       sync.Mutex
	provider *gooidc.Provider
	oauth    *oauth2.Config
	verifier *gooidc.IDTokenVerifier
}

// ErrNotConfigured — вход через ФСП ID не настроен.
var ErrNotConfigured = errors.New("oidc: provider not configured")

// New создаёт клиент. Пустой Issuer означает «не настроен».
func New(cfg Config) *Client {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{gooidc.ScopeOpenID, "profile", "email", "fsp.achievements"}
	}
	return &Client{cfg: cfg}
}

// Enabled сообщает, настроен ли провайдер.
func (c *Client) Enabled() bool { return c != nil && c.cfg.Issuer != "" }

func (c *Client) ensure(ctx context.Context) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return nil
	}
	p, err := gooidc.NewProvider(ctx, c.cfg.Issuer)
	if err != nil {
		return fmt.Errorf("oidc: discovery %s: %w", c.cfg.Issuer, err)
	}
	c.provider = p
	c.oauth = &oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  c.cfg.RedirectURL,
		Scopes:       c.cfg.Scopes,
	}
	c.verifier = p.Verifier(&gooidc.Config{ClientID: c.cfg.ClientID})
	return nil
}

// AuthURL строит адрес авторизации с PKCE (S256) и nonce.
func (c *Client) AuthURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	if err := c.ensure(ctx); err != nil {
		return "", err
	}
	return c.oauth.AuthCodeURL(state, gooidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

// Exchange меняет код на токены, проверяет id_token (подпись, issuer, aud, срок, nonce).
func (c *Client) Exchange(ctx context.Context, code, verifier, nonce string) (Identity, error) {
	if err := c.ensure(ctx); err != nil {
		return Identity{}, err
	}
	tok, err := c.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("oidc: exchange: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return Identity{}, errors.New("oidc: id_token missing")
	}
	idt, err := c.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("oidc: verify id_token: %w", err)
	}
	if nonce != "" && idt.Nonce != nonce {
		return Identity{}, errors.New("oidc: nonce mismatch")
	}
	var claims struct {
		FSPID         string `json:"fsp_id"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("oidc: claims: %w", err)
	}
	if claims.FSPID == "" {
		// Реальный Keycloak ФСП может отдавать идентификатор под другим именем; до договорённости — sub.
		claims.FSPID = idt.Subject
	}
	return Identity{
		Issuer: idt.Issuer, Subject: idt.Subject, FSPID: claims.FSPID,
		Email: claims.Email, EmailVerified: claims.EmailVerified,
		Name: claims.Name, GivenName: claims.GivenName, FamilyName: claims.FamilyName,
		AccessToken: tok.AccessToken, Expiry: tok.Expiry,
	}, nil
}

// Issuer возвращает настроенный issuer.
func (c *Client) Issuer() string { return c.cfg.Issuer }

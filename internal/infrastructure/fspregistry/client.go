// Package fspregistry — клиент реестра достижений ФСП. Контракт API — наше
// предположение (docs/05-fsp-integration.md); адаптер изолирует платформу от будущих изменений.
package fspregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Achievement — запись реестра.
type Achievement struct {
	ExternalID string `json:"external_id"`
	Discipline string `json:"discipline"`
	Level      string `json:"level"`
	EventName  string `json:"event_name"`
	EventDate  string `json:"event_date"` // YYYY-MM-DD
	ResultKind string `json:"result_kind"`
	Place      *int   `json:"place,omitempty"`
	RankTitle  string `json:"rank_title,omitempty"`
	Rating     *int   `json:"rating,omitempty"`
	Role       string `json:"role,omitempty"`
	TeamName   string `json:"team_name,omitempty"`
}

// Registry — интерфейс для use case (mock и реальный реестр взаимозаменяемы).
type Registry interface {
	Achievements(ctx context.Context, fspID, accessToken string) ([]Achievement, error)
}

// Client — HTTP-клиент реестра.
type Client struct {
	baseURL string
	http    *http.Client
}

// ErrNotConfigured — адрес реестра не задан.
var ErrNotConfigured = errors.New("fspregistry: not configured")

// New создаёт клиент; пустой baseURL означает «не настроен».
func New(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 10 * time.Second}}
}

// Enabled сообщает, задан ли адрес.
func (c *Client) Enabled() bool { return c != nil && c.baseURL != "" }

// Achievements запрашивает достижения участника от его имени (Bearer — access-токен ФСП ID).
func (c *Client) Achievements(ctx context.Context, fspID, accessToken string) ([]Achievement, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	u := c.baseURL + "/registry/v1/participants/" + url.PathEscape(fspID) + "/achievements"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fspregistry: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return []Achievement{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fspregistry: status %d", resp.StatusCode)
	}
	var body struct {
		Items []Achievement `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("fspregistry: decode: %w", err)
	}
	if body.Items == nil {
		body.Items = []Achievement{}
	}
	return body.Items, nil
}

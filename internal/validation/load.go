package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// LoadResult — результаты нагрузочного прогона.
type LoadResult struct {
	BaseURL     string
	Users       int
	Duration    time.Duration
	Rows        []LoadRow
	TotalReqs   int
	TotalErrors int
	RPS         float64
}

// LoadRow — метрики одной операции.
type LoadRow struct {
	Name   string
	Count  int
	Errors int         // транспортные ошибки и любые ответы не 2xx (в том числе 401, 404, 429)
	Codes  map[int]int // распределение кодов ответа, не равных 200
	P50    time.Duration
	P95    time.Duration
	P99    time.Duration
	Max    time.Duration
}

type loadOp struct {
	name string
	do   func(ctx context.Context, c *http.Client) (int, error)
}

// Load бьёт по локальному стенду users параллельными «пользователями» duration секунд:
// публичные операции и, если задан логин работодателя, карта категорий и построение подборки.
func Load(ctx context.Context, baseURL string, users int, duration time.Duration, employerEmail, password string) (LoadResult, error) {
	// Пул соединений под число пользователей: иначе на Windows быстро кончаются эфемерные порты.
	transport := &http.Transport{MaxIdleConns: users * 2, MaxIdleConnsPerHost: users * 2, MaxConnsPerHost: users * 2, IdleConnTimeout: 60 * time.Second}
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport}
	defer transport.CloseIdleConnections()
	get := func(path string, token string) func(context.Context, *http.Client) (int, error) {
		return func(ctx context.Context, c *http.Client) (int, error) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := c.Do(req)
			if err != nil {
				return 0, err
			}
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
			return resp.StatusCode, nil
		}
	}
	ops := []loadOp{
		{"GET /healthz", get("/healthz", "")},
		{"GET /api/v1/vacancies", get("/api/v1/vacancies?limit=20", "")},
		{"GET /api/v1/refdata", get("/api/v1/refdata", "")},
	}
	if employerEmail != "" {
		token, needID, err := loginEmployer(ctx, client, baseURL, employerEmail, password)
		if err != nil {
			return LoadResult{}, fmt.Errorf("login employer: %w", err)
		}
		ops = append(ops, loadOp{"GET /employer/needs", get("/api/v1/employer/needs", token)})
		if needID != "" {
			// Одно построение подборки — чтобы было что читать; в самом прогоне только чтение (без тысяч версий в базе).
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/employer/needs/"+needID+"/selection", bytes.NewBufferString(`{"limit":30}`))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			if resp, err := client.Do(req); err == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
			ops = append(ops, loadOp{"GET /employer/needs/{id}/map", get("/api/v1/employer/needs/"+needID+"/map", token)})
			ops = append(ops, loadOp{"GET /employer/needs/{id}/selection", get("/api/v1/employer/needs/"+needID+"/selection", token)})
			ops = append(ops, loadOp{"GET /employer/candidates/search", get("/api/v1/employer/candidates/search?specialization=backend&limit=30", token)})
		}
	}
	type sample struct {
		op   int
		dur  time.Duration
		code int
		err  bool
	}
	var mu sync.Mutex
	var samples []sample
	deadline := time.Now().Add(duration)
	var wg sync.WaitGroup
	for u := 0; u < users; u++ {
		wg.Add(1)
		go func(u int) {
			defer wg.Done()
			i := u
			for time.Now().Before(deadline) && ctx.Err() == nil {
				op := ops[i%len(ops)]
				i++
				start := time.Now()
				code, err := op.do(ctx, client)
				d := time.Since(start)
				mu.Lock()
				// Ошибка — всё, кроме 2xx: быстрые 401/404/429 иначе выглядели бы как «успешные» и
				// завышали пропускную способность (так было в первой версии прогона).
				samples = append(samples, sample{op: (i - 1) % len(ops), dur: d, code: code, err: err != nil || code < 200 || code >= 300})
				mu.Unlock()
			}
		}(u)
	}
	wg.Wait()
	res := LoadResult{BaseURL: baseURL, Users: users, Duration: duration}
	for oi, op := range ops {
		var ds []time.Duration
		errs := 0
		codes := map[int]int{}
		for _, s := range samples {
			if s.op != oi {
				continue
			}
			ds = append(ds, s.dur)
			if s.err {
				errs++
			}
			if s.code != http.StatusOK {
				codes[s.code]++
			}
		}
		if len(ds) == 0 {
			continue
		}
		sort.Slice(ds, func(a, b int) bool { return ds[a] < ds[b] })
		q := func(p float64) time.Duration { return ds[int(float64(len(ds)-1)*p)] }
		res.Rows = append(res.Rows, LoadRow{Name: op.name, Count: len(ds), Errors: errs, Codes: codes, P50: q(0.5), P95: q(0.95), P99: q(0.99), Max: ds[len(ds)-1]})
		res.TotalReqs += len(ds)
		res.TotalErrors += errs
	}
	res.RPS = float64(res.TotalReqs) / duration.Seconds()
	return res, nil
}

func loginEmployer(ctx context.Context, c *http.Client, baseURL, email, password string) (token, needID string, err error) {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/employer/needs", nil)
	req2.Header.Set("Authorization", "Bearer "+out.AccessToken)
	resp2, err := c.Do(req2)
	if err != nil {
		return out.AccessToken, "", nil //nolint:nilerr // карта и подборка просто не попадут в прогон
	}
	defer func() { _ = resp2.Body.Close() }()
	var needs struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&needs)
	if len(needs.Items) > 0 {
		needID = needs.Items[0].ID
	}
	return out.AccessToken, needID, nil
}

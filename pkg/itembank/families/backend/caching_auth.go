package backend

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(lruMisses{})
	itembank.Register(ttlOriginHits{})
	itembank.Register(jwtClaimsCheck{})
	itembank.Register(corsOutcome{})
}

// lruMisses — сколько промахов (или попаданий) даст LRU-кэш на последовательности обращений.
type lruMisses struct{}

func (lruMisses) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.caching.lru-misses.v1", Spec: itembank.SpecBackend, Topic: "caching",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.0, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"cache", "lru"},
	}
}

func (f lruMisses) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xCA01)
	capacity := itembank.IntBetween(r, 2, 3)
	prefix := itembank.Pick(r, []string{"user", "order", "product", "session", "price"})
	keyPool := make([]string, 0, 5)
	for _, id := range itembank.PickN(r, []int{3, 7, 12, 21, 42, 58, 73, 91}, 5) {
		keyPool = append(keyPool, fmt.Sprintf("%s:%d", prefix, id))
	}
	keys := itembank.PickN(r, keyPool, itembank.IntBetween(r, 4, 5))
	n := itembank.IntBetween(r, 8, 10)
	seq := make([]string, n)
	for i := range seq {
		seq[i] = itembank.Pick(r, keys)
	}
	// Симуляция LRU: cache хранит ключи от самого давнего к самому свежему.
	var cache []string
	misses := 0
	var trace []string
	for _, k := range seq {
		idx := -1
		for i, c := range cache {
			if c == k {
				idx = i
			}
		}
		if idx >= 0 {
			cache = append(append(append([]string{}, cache[:idx]...), cache[idx+1:]...), k)
			trace = append(trace, k+" ✓")
			continue
		}
		misses++
		if len(cache) == capacity {
			trace = append(trace, fmt.Sprintf("%s ✗ (вытеснен %s)", k, cache[0]))
			cache = cache[1:]
		} else {
			trace = append(trace, k+" ✗")
		}
		cache = append(cache, k)
	}
	askHits := r.IntN(2) == 0
	answer := misses
	what := "промахов"
	if askHits {
		answer = n - misses
		what = "попаданий"
	}
	stems := []string{
		fmt.Sprintf("Кэш работает по стратегии LRU и вмещает %d записи; изначально он пуст. Обращения по ключам идут в порядке: %s. Сколько %s будет при этих обращениях?", capacity, strings.Join(seq, ", "), what),
		fmt.Sprintf("LRU-кэш ёмкостью %d (пустой на старте) обслуживает последовательность запросов: %s. Укажите число %s.", capacity, strings.Join(seq, ", "), what),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("LRU вытесняет ключ, к которому дольше всего не обращались; попадание делает ключ самым свежим. Трассировка (✓ попадание, ✗ промах): %s. "+
			"Промахов %d, попаданий %d. Типичная ошибка — вытеснять самый ранний добавленный ключ (FIFO), не обновляя свежесть при попадании.",
			strings.Join(trace, ", "), misses, n-misses),
	}, nil
}

// ttlOriginHits — сколько запросов дойдут до origin при кэшировании по max-age.
type ttlOriginHits struct{}

func (ttlOriginHits) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.caching.ttl-origin-hits.v1", Spec: itembank.SpecBackend, Topic: "caching",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.6, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"cache", "http"},
	}
}

func (f ttlOriginHits) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xCA02)
	ttl := itembank.Pick(r, []int{20, 30, 45, 60, 90, 120})
	path := "/" + itembank.Ident(r, "table") + "/" + strconv.Itoa(itembank.IntBetween(r, 1, 99))
	n := itembank.IntBetween(r, 6, 8)
	times := make([]int, n)
	t := 0
	for i := range times {
		t += itembank.IntBetween(r, 1, 6) * ttl / 4
		times[i] = t
	}
	storedAt := -1
	origin := 0
	var trace []string
	for _, t := range times {
		if storedAt >= 0 && t-storedAt < ttl {
			trace = append(trace, fmt.Sprintf("%d с — из кэша (возраст %d)", t, t-storedAt))
			continue
		}
		origin++
		storedAt = t
		trace = append(trace, fmt.Sprintf("%d с — origin", t))
	}
	var parts []string
	for _, t := range times {
		parts = append(parts, strconv.Itoa(t))
	}
	stems := []string{
		fmt.Sprintf("Прокси кэширует ответ `GET %s` по заголовку `Cache-Control: max-age=%d`; ответ свежий, пока его возраст строго меньше max-age, иначе прокси идёт к origin и сохраняет новый ответ. Кэш пуст; запросы приходят в моменты (секунды): %s. Сколько запросов дойдут до origin?", path, ttl, strings.Join(parts, ", ")),
		fmt.Sprintf("Сервис кэширует результат `GET %s` на %d секунд с момента получения от origin (ровно через %d секунд запись считается устаревшей и перезапрашивается). Кэш изначально пуст. Запросы поступают в секунды %s. Сколько раз будет вызван origin?", path, ttl, ttl, strings.Join(parts, ", ")),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Answer: itembank.Answer{Number: float64(origin)},
		Explanation: fmt.Sprintf("Отсчёт свежести идёт от момента последнего ответа origin, а не от предыдущего запроса. Трассировка: %s. Итого обращений к origin — %d. "+
			"Типичная ошибка — продлевать срок жизни при каждом попадании или считать ответ свежим при возрасте, равном max-age.",
			strings.Join(trace, "; "), origin),
	}, nil
}

// jwtClaimsCheck — результат проверки claims JWT.
type jwtClaimsCheck struct{}

func (jwtClaimsCheck) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.auth_security.jwt-claims-check.v1", Spec: itembank.SpecBackend, Topic: "auth_security",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.1, C: 0.2}, TimeLimit: 120 * time.Second, Tags: []string{"jwt", "auth"},
	}
}

var jwtOutcomes = []string{
	"Токен отклонён: срок действия истёк (exp)",
	"Токен отклонён: ещё не вступил в силу (nbf)",
	"Токен отклонён: неверный издатель (iss)",
	"Токен отклонён: неверная аудитория (aud)",
	"Токен принят: все claims проходят проверку",
}

func (f jwtClaimsCheck) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xA001)
	services := itembank.Idents(r, "service", 2)
	svc, other := strings.ReplaceAll(services[0], "-", ""), strings.ReplaceAll(services[1], "-", "")
	expectIss := "https://auth." + svc + ".example"
	expectAud := svc + "-api"
	leeway := itembank.Pick(r, []int{0, 30, 60})
	now := 1700000000 + itembank.IntBetween(r, 1, 9000)*1000 + itembank.IntBetween(r, 0, 999)
	failure := r.IntN(5) // индекс в jwtOutcomes; 4 — валиден
	iss, aud := expectIss, expectAud
	exp := now + itembank.IntBetween(r, 300, 3600)
	nbf := now - itembank.IntBetween(r, 0, 600)
	switch failure {
	case 0:
		exp = now - itembank.IntBetween(r, leeway+1, leeway+300)
		nbf = exp - itembank.IntBetween(r, 300, 3600)
	case 1:
		nbf = now + itembank.IntBetween(r, leeway+1, leeway+120)
		exp = nbf + itembank.IntBetween(r, 300, 3600)
	case 2:
		iss = "https://auth." + other + ".example"
	case 3:
		aud = other + "-api"
	}
	// Ловушка для валидных по времени случаев: просрочка или ранний старт внутри leeway
	// (токен при этом остаётся согласованным: nbf < exp).
	if failure >= 2 && leeway > 1 {
		switch r.IntN(3) {
		case 0:
			exp = now - itembank.IntBetween(r, 1, leeway-1)
			nbf = now - itembank.IntBetween(r, 300, 600)
		case 1:
			nbf = now + itembank.IntBetween(r, 1, leeway-1)
		}
	}
	// Ответ вычисляется независимо от сценария: порядок проверок exp → nbf → iss → aud.
	var computed int
	switch {
	case now >= exp+leeway:
		computed = 0
	case now < nbf-leeway:
		computed = 1
	case iss != expectIss:
		computed = 2
	case aud != expectAud:
		computed = 3
	default:
		computed = 4
	}
	payload := fmt.Sprintf("{\n  \"sub\": \"%s\",\n  \"iss\": \"%s\",\n  \"aud\": \"%s\",\n  \"iat\": %d,\n  \"nbf\": %d,\n  \"exp\": %d\n}",
		itembank.Ident(r, "name_lat"), iss, aud, nbf-itembank.IntBetween(r, 0, 60), nbf, exp)
	var distractors []string
	for i, o := range jwtOutcomes {
		if i != computed {
			distractors = append(distractors, o)
		}
	}
	options, key := itembank.ShuffledOptions(r, jwtOutcomes[computed], distractors...)
	checks := fmt.Sprintf("exp: now %d против exp + leeway = %d; nbf: now против nbf − leeway = %d; iss ожидается %s; aud ожидается %s.",
		now, exp+leeway, nbf-leeway, expectIss, expectAud)
	stems := []string{
		fmt.Sprintf("Сервер проверил подпись JWT (она верна) и валидирует claims в порядке exp → nbf → iss → aud, останавливаясь на первой ошибке. Время сервера now = %d (unix), ожидаемые iss = `%s`, aud = `%s`, допуск на рассинхрон часов (leeway) = %d с: токен действителен, пока now < exp + leeway и now ≥ nbf − leeway. Каков результат проверки?", now, expectIss, expectAud, leeway),
		fmt.Sprintf("Middleware аутентификации получил JWT с корректной подписью. Настройки: now = %d, iss = `%s`, aud = `%s`, leeway = %d с (exp и nbf сравниваются с учётом допуска: now < exp + leeway, now ≥ nbf − leeway); проверки идут в порядке exp, nbf, iss, aud до первой неудачи. Что решит middleware?", now, expectIss, expectAud, leeway),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "json", Source: payload},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: "Проверки: " + checks + " Первое нарушение определяет результат, итог — «" + jwtOutcomes[computed] + "». " +
			"Типичная ошибка — забыть про leeway (токен, просроченный на секунды внутри допуска, ещё валиден) или проверять iss/aud до временных claims.",
	}, nil
}

// corsOutcome — что сделает браузер с кросс-доменным запросом при заданной CORS-настройке.
type corsOutcome struct{}

func (corsOutcome) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.auth_security.cors-outcome.v1", Spec: itembank.SpecBackend, Topic: "auth_security",
		Grade: itembank.GradeSenior, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.8, C: 0.25}, TimeLimit: 150 * time.Second, Tags: []string{"http", "cors", "browser"},
	}
}

var corsOutcomes = []string{
	"Preflight не отправляется; ответ сервера доступен скрипту",
	"Preflight не отправляется; запрос уходит, но браузер не даёт скрипту прочитать ответ",
	"Preflight (OPTIONS) проходит, затем выполняется основной запрос",
	"Preflight отклонён: origin не входит в allowed_origins",
	"Preflight отклонён: метод не входит в allowed_methods",
	"Preflight отклонён: заголовок запроса не входит в allowed_headers",
}

type corsHeader struct {
	line   string
	name   string
	simple bool
}

var corsHeaders = []corsHeader{
	{"Content-Type: application/json", "Content-Type", false},
	{"Content-Type: text/plain", "Content-Type", true},
	{"Authorization: Bearer <token>", "Authorization", false},
	{"X-Request-Id: <uuid>", "X-Request-Id", false},
	{"Accept: application/json", "Accept", true},
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func (f corsOutcome) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xA002)
	svc := strings.ReplaceAll(itembank.Ident(r, "service"), "-", "")
	origin := itembank.Pick(r, []string{"https://app." + svc + ".example", "https://admin." + svc + ".example", "https://shop." + svc + ".example", "http://localhost:3000"})
	otherOrigin := "https://" + itembank.Pick(r, []string{"partner.", "www.", "staging."}) + svc + ".example"
	method := itembank.Pick(r, []string{"GET", "POST", "POST", "PUT", "PATCH", "DELETE"})
	path := "/api/" + itembank.Ident(r, "table")
	hdrs := itembank.PickN(r, corsHeaders, itembank.IntBetween(r, 0, 2))
	// Два Content-Type одновременно не бывает.
	seenCT := false
	var reqHeaders []corsHeader
	for _, h := range hdrs {
		if h.name == "Content-Type" {
			if seenCT {
				continue
			}
			seenCT = true
		}
		reqHeaders = append(reqHeaders, h)
	}
	safeMethod := method == "GET" || method == "POST"
	simple := safeMethod
	for _, h := range reqHeaders {
		simple = simple && h.simple
	}
	failure := r.IntN(4) // 0 — нет отказа, 1 — origin, 2 — метод, 3 — заголовок
	allowedOrigins := []string{origin}
	if r.IntN(2) == 0 {
		allowedOrigins = append(allowedOrigins, otherOrigin)
	}
	if failure == 1 {
		allowedOrigins = []string{otherOrigin}
	}
	allowedMethods := []string{"GET", "POST"}
	if !safeMethod && failure != 2 {
		allowedMethods = append(allowedMethods, method)
	}
	if failure != 2 && r.IntN(2) == 0 {
		for _, extra := range []string{"PUT", "DELETE"} {
			if !contains(allowedMethods, extra) {
				allowedMethods = append(allowedMethods, extra)
			}
		}
	}
	allowedHeaders := []string{"Content-Type"}
	for _, h := range reqHeaders {
		if !contains(allowedHeaders, h.name) {
			allowedHeaders = append(allowedHeaders, h.name)
		}
	}
	if failure == 3 {
		var nonSimple []string
		for _, h := range reqHeaders {
			if !h.simple && !contains(nonSimple, h.name) {
				nonSimple = append(nonSimple, h.name)
			}
		}
		if len(nonSimple) > 0 {
			drop := itembank.Pick(r, nonSimple)
			var kept []string
			for _, h := range allowedHeaders {
				if h != drop {
					kept = append(kept, h)
				}
			}
			allowedHeaders = kept
		}
	}
	// Ответ вычисляется по правилам CORS независимо от сценария генерации.
	originOK := contains(allowedOrigins, origin)
	var computed int
	switch {
	case simple && originOK:
		computed = 0
	case simple:
		computed = 1
	case !originOK:
		computed = 3
	case !safeMethod && !contains(allowedMethods, method):
		computed = 4
	default:
		computed = 2
		for _, h := range reqHeaders {
			if !h.simple && !contains(allowedHeaders, h.name) {
				computed = 5
			}
		}
	}
	quote := func(xs []string) string {
		q := make([]string, len(xs))
		for i, x := range xs {
			q[i] = "\"" + x + "\""
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	cfg := fmt.Sprintf("cors:\n  allowed_origins: %s\n  allowed_methods: %s\n  allowed_headers: %s", quote(allowedOrigins), quote(allowedMethods), quote(allowedHeaders))
	var hdrLines []string
	for _, h := range reqHeaders {
		hdrLines = append(hdrLines, h.line)
	}
	request := fmt.Sprintf("%s https://api.%s.example%s", method, svc, path)
	if len(hdrLines) > 0 {
		request += "\n" + strings.Join(hdrLines, "\n")
	}
	var distractors []string
	for i, o := range corsOutcomes {
		if i != computed {
			distractors = append(distractors, o)
		}
	}
	options, key := itembank.ShuffledOptions(r, corsOutcomes[computed], itembank.PickN(r, distractors, 3)...)
	simpleText := "запрос не «простой» (нестандартный метод или заголовок вне safelist), поэтому браузер сначала шлёт OPTIONS"
	if simple {
		simpleText = "запрос «простой» (GET/POST и только safelisted-заголовки), поэтому preflight не нужен, а CORS проверяется по ответу"
	}
	stems := []string{
		fmt.Sprintf("Скрипт на странице с origin `%s` выполняет `fetch`, формирующий такой запрос:\n\n```http\n%s\n```\n\nСервер отвечает по настройке CORS ниже (preflight проверяет origin, затем метод, затем заголовки). Что произойдёт в браузере?", origin, request),
		fmt.Sprintf("Фронтенд, открытый с `%s`, отправляет из JavaScript запрос:\n\n```http\n%s\n```\n\nНа API стоит CORS-middleware с конфигурацией ниже; preflight валидирует origin, потом метод, потом заголовки. Как поведёт себя браузер?", origin, request),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "yaml", Source: cfg},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: fmt.Sprintf("Сначала решается, нужен ли preflight: %s. Origin %s в списке разрешённых; итог — «%s». "+
			"Типичная ошибка — считать, что Content-Type: application/json делает запрос простым, или что при неразрешённом origin простой запрос вообще не уходит на сервер.",
			simpleText, map[bool]string{true: "есть", false: "отсутствует"}[originOK], corsOutcomes[computed]),
	}, nil
}

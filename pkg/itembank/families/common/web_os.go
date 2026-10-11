package common

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(httpCacheHeaders{})
	itembank.Register(httpIdempotentMethods{})
	itembank.Register(httpRedirectCodes{})
	itembank.Register(osSchedulingOrder{})
	itembank.Register(osFileDescriptors{})
	itembank.Register(netTCPvsUDP{})
}

// httpCacheHeaders — поведение браузерного кэша по Cache-Control, Age и ETag при повторном запросе.
type httpCacheHeaders struct{}

func (httpCacheHeaders) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.http.cache-headers.v1", Spec: itembank.SpecCommon, Topic: "http",
		Grade: itembank.GradeSenior, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.8, C: 0.25}, TimeLimit: 120 * time.Second, Tags: []string{"http", "caching"},
	}
}

var cacheOutcomes = []string{
	"Браузер возьмёт ответ из своего кэша, не обращаясь к серверу",
	"Браузер отправит условный запрос с `If-None-Match`; если ресурс не изменился, сервер ответит `304 Not Modified` без тела",
	"Браузер отправит обычный (безусловный) запрос и получит полный ответ `200 OK` с телом",
	"Браузер сразу отдаст устаревший ответ из кэша и параллельно обновит его фоновым запросом к серверу",
}

func (f httpCacheHeaders) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE1E)
	const (
		oFresh = iota
		oCond
		oFull
		oSWR
	)
	var path, contentType string
	if r.IntN(2) == 0 {
		path = fmt.Sprintf("/api/%s/%d", itembank.Ident(r, "table"), itembank.IntBetween(r, 1, 999))
		contentType = "application/json"
	} else {
		path = fmt.Sprintf("/static/%s.%06x.js", itembank.Pick(r, []string{"app", "vendor", "main", "chunk"}), r.Uint32()&0xFFFFFF)
		contentType = "text/javascript"
	}
	maxAge := itembank.Pick(r, []int{60, 120, 300, 600, 900, 1800, 3600})
	kind := r.IntN(5)
	hasETag := r.IntN(4) != 0
	etag := fmt.Sprintf("%08x", r.Uint32())
	age, swr := 0, 0
	var cc string
	switch kind {
	case 0:
		cc = fmt.Sprintf("%s, max-age=%d", itembank.Pick(r, []string{"public", "private"}), maxAge)
	case 1:
		cc = itembank.Pick(r, []string{"no-cache", fmt.Sprintf("no-cache, max-age=%d", maxAge)})
	case 2:
		cc = "no-store"
	case 3:
		swr = itembank.Pick(r, []int{30, 60, 120, 300})
		cc = fmt.Sprintf("max-age=%d, stale-while-revalidate=%d", maxAge, swr)
	default:
		age = itembank.IntBetween(r, 1, maxAge/10-1) * 10
		cc = fmt.Sprintf("public, max-age=%d", maxAge)
	}
	remain := maxAge - age
	var delta int
	switch kind {
	case 1, 2:
		delta = itembank.IntBetween(r, 1, 2*maxAge)
	case 3:
		switch r.IntN(3) {
		case 0:
			delta = itembank.IntBetween(r, 1, maxAge-1)
		case 1:
			delta = itembank.IntBetween(r, maxAge, maxAge+swr-1)
		default:
			delta = itembank.IntBetween(r, maxAge+swr, maxAge+swr+300)
		}
	default:
		if r.IntN(2) == 0 {
			delta = itembank.IntBetween(r, 1, remain-1)
		} else {
			delta = itembank.IntBetween(r, remain, remain+600)
		}
	}
	staleOutcome := oFull
	staleText := "ответ устарел, валидатора нет — браузер запросит ресурс заново и получит полный ответ 200"
	if hasETag {
		staleOutcome = oCond
		staleText = "ответ устарел, но есть валидатор ETag — браузер отправит условный запрос If-None-Match и при неизменённом ресурсе получит 304 без тела"
	}
	var correct int
	var explain string
	switch {
	case kind == 2:
		correct = oFull
		explain = "no-store запрещает сохранять ответ в любом кэше, поэтому каждый запрос идёт на сервер за полным ответом; ETag здесь не помогает — сохранённой копии нет."
	case kind == 1:
		correct = staleOutcome
		explain = "no-cache не запрещает хранить ответ, но требует перепроверять его у сервера перед каждым использованием, сколько бы времени ни прошло: " + staleText + "."
	case kind == 3 && delta < maxAge:
		correct = oFresh
		explain = fmt.Sprintf("Прошло %d с < max-age=%d — ответ ещё свеж, кэш отдаёт его без запроса; окно stale-while-revalidate пока не нужно.", delta, maxAge)
	case kind == 3 && delta < maxAge+swr:
		correct = oSWR
		explain = fmt.Sprintf("max-age=%d истёк, но stale-while-revalidate=%d разрешает ещё %d с отдавать устаревший ответ сразу, обновляя его в фоне; "+
			"%d с попадает в это окно (%d…%d с).", maxAge, swr, swr, delta, maxAge, maxAge+swr-1)
	case kind == 3:
		correct = staleOutcome
		explain = fmt.Sprintf("Прошло %d с — истекли и max-age=%d, и окно stale-while-revalidate=%d (до %d с): %s.", delta, maxAge, swr, maxAge+swr, staleText)
	case age+delta < maxAge:
		correct = oFresh
		if age > 0 {
			explain = fmt.Sprintf("Заголовок Age показывает, что ответ уже провёл %d с в промежуточном кэше: остаток свежести %d − %d = %d с; прошло %d с < %d с — "+
				"ответ свеж, кэш отдаёт его без запроса.", age, maxAge, age, remain, delta, remain)
		} else {
			explain = fmt.Sprintf("Ответ свеж, пока прошедшее время меньше max-age: %d с < %d с, поэтому кэш отдаёт его без обращения к серверу.", delta, maxAge)
		}
	default:
		correct = staleOutcome
		if age > 0 {
			explain = fmt.Sprintf("Заголовок Age показывает, что ответ уже провёл %d с в промежуточном кэше: остаток свежести %d − %d = %d с; прошло %d с ≥ %d с — %s.",
				age, maxAge, age, remain, delta, remain, staleText)
		} else {
			explain = fmt.Sprintf("Прошло %d с ≥ max-age=%d: %s.", delta, maxAge, staleText)
		}
	}

	headers := []string{"HTTP/1.1 200 OK", "Cache-Control: " + cc}
	if age > 0 {
		headers = append(headers, fmt.Sprintf("Age: %d", age))
	}
	if hasETag {
		headers = append(headers, fmt.Sprintf("ETag: \"%s\"", etag))
	}
	headers = append(headers, "Content-Type: "+contentType)
	var distractors []string
	for i, o := range cacheOutcomes {
		if i != correct {
			distractors = append(distractors, o)
		}
	}
	options, key := itembank.ShuffledOptions(r, cacheOutcomes[correct], distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("В момент T браузер получил ответ на `GET %s` с заголовками из листинга. Через %d с (в момент T + %d с) пользователь снова "+
			"открывает тот же URL (обычная навигация, без принудительной перезагрузки). Как поведёт себя браузер?", path, delta, delta),
		Code:    &itembank.CodeBlock{Lang: "http", Source: strings.Join(headers, "\n")},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: explain + " Типичная ошибка — путать no-cache (хранить, но перепроверять) с no-store (не хранить) и не учитывать Age при расчёте " +
			"остатка свежести.",
	}, nil
}

// httpIdempotentMethods — какие запросы идемпотентны или безопасны.
type httpIdempotentMethods struct{}

func (httpIdempotentMethods) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.http.idempotent-methods.v1", Spec: itembank.SpecCommon, Topic: "http",
		Grade: itembank.GradeJunior, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.1, B: -0.6, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"http"},
	}
}

type httpReq struct {
	method, hint string
	idem, safe   bool
	path         func(res string, id int) string
}

func httpListPath(res string, _ int) string  { return "/" + res }
func httpItemPath(res string, id int) string { return fmt.Sprintf("/%s/%d", res, id) }

var httpReqPool = []httpReq{
	{"GET", "получить список записей", true, true, httpListPath},
	{"GET", "получить одну запись", true, true, httpItemPath},
	{"HEAD", "проверить, существует ли запись", true, true, httpItemPath},
	{"OPTIONS", "узнать поддерживаемые методы", true, true, httpListPath},
	{"PUT", "заменить запись целиком переданным представлением", true, false, httpItemPath},
	{"DELETE", "удалить запись", true, false, httpItemPath},
	{"POST", "создать новую запись", false, false, httpListPath},
	{"POST", "добавить ещё одну позицию в запись", false, false, func(res string, id int) string { return fmt.Sprintf("/%s/%d/items", res, id) }},
	{"PATCH", "увеличить счётчик записи на 1", false, false, httpItemPath},
}

func (f httpIdempotentMethods) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE1F)
	askSafe := r.IntN(2) == 0
	var yes, no []httpReq
	for _, q := range httpReqPool {
		ok := q.idem
		if askSafe {
			ok = q.safe
		}
		if ok {
			yes = append(yes, q)
		} else {
			no = append(no, q)
		}
	}
	render := func(qs []httpReq) []string {
		out := make([]string, 0, len(qs))
		for _, q := range qs {
			out = append(out, fmt.Sprintf("`%s %s` — %s", q.method, q.path(itembank.Ident(r, "table"), itembank.IntBetween(r, 10, 999)), q.hint))
		}
		return out
	}
	correct := render(itembank.PickN(r, yes, itembank.IntBetween(r, 2, 3)))
	wrong := render(itembank.PickN(r, no, itembank.IntBetween(r, 2, 3)))
	stem := "Клиент отправил запрос, но не дождался ответа из-за сетевого сбоя и собирается повторить его. Какие из перечисленных запросов идемпотентны — " +
		"их повторное выполнение оставит состояние сервера таким же, как после однократного? Выберите все подходящие."
	if askSafe {
		stem = "Какие из перечисленных запросов являются безопасными (safe) с точки зрения HTTP — они не должны изменять состояние сервера, " +
			"и их можно, например, выполнять при предзагрузке? Выберите все подходящие."
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    stem,
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: "Безопасные методы — GET, HEAD, OPTIONS: только чтение. Идемпотентные — безопасные плюс PUT и DELETE: повторный PUT записывает то же " +
			"представление, повторный DELETE оставляет ресурс удалённым (код ответа может стать 404, но состояние не меняется). POST создаёт новую запись " +
			"при каждом вызове, PATCH с относительным изменением («увеличить на 1») тоже не идемпотентен. Типичная ошибка — считать DELETE неидемпотентным " +
			"из-за разного кода ответа или PUT небезопасным и потому неидемпотентным.",
	}, nil
}

// httpRedirectCodes — какой код перенаправления подходит сценарию (постоянное/временное, сохранение метода).
type httpRedirectCodes struct{}

func (httpRedirectCodes) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.http.redirect-codes.v1", Spec: itembank.SpecCommon, Topic: "http",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.1, C: 0.25}, TimeLimit: 90 * time.Second, Tags: []string{"http"},
	}
}

type redirectScenario struct {
	code        string
	texts       []string // %[1]s — старый адрес, %[2]s — новый
	distractors []string // коды, которые в этом сценарии однозначно не подходят
	explain     string
}

var redirectScenarios = []redirectScenario{
	{
		code: "301 Moved Permanently",
		texts: []string{
			"Страница `%[1]s` навсегда перенесена на `%[2]s`; речь об обычной GET-навигации, и нужно, чтобы поисковые системы и закладки обновились на новый адрес.",
			"Сайт переехал: `%[1]s` теперь всегда должен открываться как `%[2]s` (только GET-запросы), а старый адрес больше не используется.",
		},
		distractors: []string{"302 Found", "303 See Other", "307 Temporary Redirect"},
		explain:     "Постоянное перенаправление для GET — 301: клиенты и поисковые системы запоминают новый адрес; 302/307 — временные, 303 — ответ на POST.",
	},
	{
		code: "302 Found",
		texts: []string{
			"Из-за A/B-теста часть пользователей, открывающих `%[1]s`, нужно временно отправлять на `%[2]s`; это GET, старый адрес остаётся основным, и запоминать перенаправление клиентам не следует.",
			"Страница `%[1]s` на время работ недоступна: GET-запросы временно перенаправляются на `%[2]s`, после чего всё вернётся как было.",
		},
		distractors: []string{"301 Moved Permanently", "303 See Other", "308 Permanent Redirect"},
		explain:     "Временное перенаправление GET-запроса — 302 Found: клиент не должен запоминать новый адрес; 301/308 — постоянные, 303 — ответ на POST.",
	},
	{
		code: "303 See Other",
		texts: []string{
			"После успешной обработки `POST %[1]s` сервер хочет, чтобы браузер загрузил страницу результата `%[2]s` обычным GET-запросом (паттерн POST → redirect → GET), чтобы обновление страницы не отправляло форму повторно.",
			"Форма отправлена на `POST %[1]s`; сервер создал запись и должен отправить браузер на `%[2]s`, причём туда нужно идти именно методом GET.",
		},
		distractors: []string{"301 Moved Permanently", "307 Temporary Redirect", "308 Permanent Redirect"},
		explain:     "303 See Other прямо предписывает перейти на другой ресурс методом GET — это стандартный ответ на POST; 307/308 сохранили бы POST, 301 — постоянный перенос самого ресурса.",
	},
	{
		code: "307 Temporary Redirect",
		texts: []string{
			"Эндпоинт `POST %[1]s` временно обслуживается по адресу `%[2]s`; клиент должен повторить тот же POST с тем же телом на новый адрес, а в будущем снова использовать старый.",
			"На время миграции запросы `POST %[1]s` нужно перенаправить на `%[2]s`, не теряя метод и тело запроса; перенаправление временное.",
		},
		distractors: []string{"301 Moved Permanently", "302 Found", "308 Permanent Redirect"},
		explain:     "307 — временное перенаправление с обязательным сохранением метода и тела; 302 позволяет клиенту сменить метод на GET, 301/308 — постоянные.",
	},
	{
		code: "308 Permanent Redirect",
		texts: []string{
			"Эндпоинт `POST %[1]s` навсегда перенесён на `%[2]s`; клиенты должны повторить запрос тем же методом с тем же телом и запомнить новый адрес.",
			"API переехал: `POST %[1]s` теперь постоянно живёт по адресу `%[2]s`, и при перенаправлении метод POST и тело должны сохраняться.",
		},
		distractors: []string{"301 Moved Permanently", "302 Found", "307 Temporary Redirect"},
		explain:     "308 — постоянное перенаправление с сохранением метода и тела; 301 допускает смену метода на GET, 302/307 — временные.",
	},
}

func (f httpRedirectCodes) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE20)
	sc := itembank.Pick(r, redirectScenarios)
	res := itembank.Ident(r, "table")
	oldPath := itembank.Pick(r, []string{"/" + res, "/v1/" + res, "/api/" + res, "/old/" + res})
	var newPath string
	switch r.IntN(3) {
	case 0:
		newPath = "/v2/" + res
	case 1:
		newPath = fmt.Sprintf("https://%s.example.com/%s", itembank.Ident(r, "service"), res)
	default:
		newPath = fmt.Sprintf("/%s/%d", res, itembank.IntBetween(r, 100, 999))
	}
	text := fmt.Sprintf(itembank.Pick(r, sc.texts), oldPath, newPath)
	options, key := itembank.ShuffledOptions(r, sc.code, sc.distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    text + " Какой код состояния должен вернуть сервер?",
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: sc.explain + " Правило: 301/302 — «классические» перенаправления, при которых клиент может сменить метод на GET; 307/308 — их аналоги " +
			"с обязательным сохранением метода и тела; 303 — явное «перейди GET-ом на другой ресурс».",
	}, nil
}

// osSchedulingOrder — порядок завершения процессов при SJF, SRTF, приоритетном планировании и Round Robin.
type osSchedulingOrder struct{}

func (osSchedulingOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.os_processes.scheduling-order.v1", Spec: itembank.SpecCommon, Topic: "os_processes",
		Grade: itembank.GradeSenior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.3, B: 0.9, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"scheduling"},
	}
}

type schedProc struct {
	name                 string
	arrival, burst, prio int
}

// schedResult — результат моделирования: порядок завершения, моменты завершения, отрезки выполнения.
type schedResult struct {
	order  []int
	finish []int
	segs   []string
}

func (s *schedResult) segment(from, to int, p schedProc) {
	s.segs = append(s.segs, fmt.Sprintf("%d–%d: %s", from, to, p.name))
}

// schedNonPreemptive — невытесняющее планирование с функцией предпочтения less (среди прибывших).
func schedNonPreemptive(ps []schedProc, less func(a, b schedProc) bool) schedResult {
	res := schedResult{finish: make([]int, len(ps))}
	done := make([]bool, len(ps))
	t := 0
	for len(res.order) < len(ps) {
		best := -1
		for i, p := range ps {
			if done[i] || p.arrival > t {
				continue
			}
			if best < 0 || less(p, ps[best]) {
				best = i
			}
		}
		if best < 0 {
			// Процессор простаивает до ближайшего прибытия.
			next := -1
			for i, p := range ps {
				if !done[i] && (next < 0 || p.arrival < ps[next].arrival) {
					next = i
				}
			}
			t = ps[next].arrival
			continue
		}
		res.segment(t, t+ps[best].burst, ps[best])
		t += ps[best].burst
		done[best] = true
		res.finish[best] = t
		res.order = append(res.order, best)
	}
	return res
}

// schedSRTF — вытесняющий SJF по наименьшему остатку; при равенстве текущий процесс не вытесняется.
func schedSRTF(ps []schedProc) schedResult {
	res := schedResult{finish: make([]int, len(ps))}
	rem := make([]int, len(ps))
	for i, p := range ps {
		rem[i] = p.burst
	}
	t, running, segStart := 0, -1, 0
	better := func(i, b int) bool {
		switch {
		case b < 0:
			return true
		case rem[i] != rem[b]:
			return rem[i] < rem[b]
		case i == running:
			return true
		case b == running:
			return false
		case ps[i].arrival != ps[b].arrival:
			return ps[i].arrival < ps[b].arrival
		default:
			return i < b
		}
	}
	for len(res.order) < len(ps) {
		best := -1
		for i, p := range ps {
			if rem[i] > 0 && p.arrival <= t && better(i, best) {
				best = i
			}
		}
		if best < 0 {
			t++
			continue
		}
		if best != running {
			if running >= 0 {
				res.segment(segStart, t, ps[running])
			}
			running, segStart = best, t
		}
		rem[best]--
		t++
		if rem[best] == 0 {
			res.segment(segStart, t, ps[best])
			res.finish[best] = t
			res.order = append(res.order, best)
			running = -1
		}
	}
	return res
}

// schedRR — Round Robin: прибывшие за квант (включая момент его окончания) встают в очередь раньше вытесненного.
func schedRR(ps []schedProc, quantum int) schedResult {
	res := schedResult{finish: make([]int, len(ps))}
	rem := make([]int, len(ps))
	byArrival := make([]int, len(ps))
	for i, p := range ps {
		rem[i] = p.burst
		byArrival[i] = i
	}
	sort.SliceStable(byArrival, func(a, b int) bool { return ps[byArrival[a]].arrival < ps[byArrival[b]].arrival })
	t, next := 0, 0
	var queue []int
	enqueue := func() {
		for next < len(ps) && ps[byArrival[next]].arrival <= t {
			queue = append(queue, byArrival[next])
			next++
		}
	}
	enqueue()
	for len(res.order) < len(ps) {
		if len(queue) == 0 {
			t = ps[byArrival[next]].arrival
			enqueue()
			continue
		}
		p := queue[0]
		queue = queue[1:]
		run := min(quantum, rem[p])
		res.segment(t, t+run, ps[p])
		t += run
		rem[p] -= run
		enqueue()
		if rem[p] == 0 {
			res.finish[p] = t
			res.order = append(res.order, p)
		} else {
			queue = append(queue, p)
		}
	}
	return res
}

func (f osSchedulingOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE21)
	n := itembank.IntBetween(r, 4, 5)
	arr := distinctInts(r, n-1, 1, 7)
	sort.Ints(arr)
	bursts := distinctInts(r, n, 1, 8)
	prios := r.Perm(n)
	ps := make([]schedProc, n)
	for i := range ps {
		ps[i] = schedProc{name: fmt.Sprintf("P%d", i+1), burst: bursts[i], prio: prios[i] + 1}
		if i > 0 {
			ps[i].arrival = arr[i-1]
		}
	}
	alg := r.IntN(4)
	quantum := itembank.Pick(r, []int{2, 3})
	var res schedResult
	var algText, rules, pitfall string
	switch alg {
	case 0:
		res = schedNonPreemptive(ps, func(a, b schedProc) bool { return a.burst < b.burst })
		algText = "невытесняющий SJF (Shortest Job First)"
		rules = "При освобождении процессора выбирается уже прибывший процесс с наименьшей длительностью; начавшийся процесс выполняется до конца."
		pitfall = "выбирать самый короткий процесс среди всех, включая ещё не прибывшие, или вытеснять уже начавшийся процесс"
	case 1:
		res = schedSRTF(ps)
		algText = "вытесняющий SJF (SRTF — по наименьшему остатку)"
		rules = "В каждый момент выполняется прибывший процесс с наименьшим остатком работы; при равенстве остатков текущий процесс не вытесняется."
		pitfall = "забывать о вытеснении при прибытии более короткого процесса или сравнивать полную длительность вместо остатка"
	case 2:
		res = schedNonPreemptive(ps, func(a, b schedProc) bool { return a.prio < b.prio })
		algText = "невытесняющий приоритетный"
		rules = "При освобождении процессора выбирается прибывший процесс с наивысшим приоритетом (меньшее число — выше приоритет); начавшийся процесс выполняется до конца."
		pitfall = "выбирать по приоритету среди всех процессов, включая ещё не прибывшие, или считать большее число более высоким приоритетом"
	default:
		res = schedRR(ps, quantum)
		algText = fmt.Sprintf("Round Robin с квантом %d", quantum)
		rules = "Готовые процессы стоят в очереди FIFO; процесс получает квант, после чего, если не завершился, встаёт в конец очереди. " +
			"Процесс, прибывший в момент окончания чужого кванта, ставится в очередь раньше вытесненного."
		pitfall = "ставить вытесненный процесс в очередь раньше только что прибывшего или не возвращать его в конец очереди"
	}
	headers := []string{"Процесс", "Прибытие", "Длительность"}
	if alg == 2 {
		headers = append(headers, "Приоритет")
	}
	rows := make([][]string, n)
	for i, p := range ps {
		rows[i] = []string{p.name, strconv.Itoa(p.arrival), strconv.Itoa(p.burst)}
		if alg == 2 {
			rows[i] = append(rows[i], strconv.Itoa(p.prio))
		}
	}
	inOrder := make([]string, n)
	finishes := make([]string, n)
	for i, idx := range res.order {
		p := ps[idx]
		inOrder[i] = fmt.Sprintf("%s (прибытие %d, длительность %d)", p.name, p.arrival, p.burst)
		if alg == 2 {
			inOrder[i] = fmt.Sprintf("%s (прибытие %d, длительность %d, приоритет %d)", p.name, p.arrival, p.burst, p.prio)
		}
		finishes[i] = fmt.Sprintf("%s — %d", p.name, res.finish[idx])
	}
	options, order := itembank.OrderingOptions(r, inOrder)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Однопроцессорная система; процессы, моменты их прибытия и длительность (в единицах времени):\n\n%s\nПланировщик — %s. %s "+
			"В каком порядке процессы завершат выполнение? Расположите от первого завершившегося к последнему.", mdTable(headers, rows), algText, rules),
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: fmt.Sprintf("Диаграмма выполнения: %s. Моменты завершения: %s. Типичная ошибка — %s.",
			strings.Join(res.segs, ", "), strings.Join(finishes, ", "), pitfall),
	}, nil
}

// osFileDescriptors — номера файловых дескрипторов после open/close/dup: правило наименьшего свободного.
type osFileDescriptors struct{}

func (osFileDescriptors) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.os_processes.fd-lowest.v1", Spec: itembank.SpecCommon, Topic: "os_processes",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.0, B: -0.4, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"linux", "posix"},
	}
}

func (f osFileDescriptors) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE22)
	files := itembank.PickN(r, []string{"app.log", "data.csv", "config.yaml", "input.txt", "cache.db", "report.pdf", "notes.md", "metrics.json"}, 6)
	varNames := []string{"a", "b", "c", "d", "e", "f", "g"}
	open := map[int]bool{0: true, 1: true, 2: true}
	lowest := func() int {
		for i := 0; ; i++ {
			if !open[i] {
				return i
			}
		}
	}
	type fdVar struct {
		name string
		fd   int
	}
	var live []fdVar // переменные с ещё открытыми дескрипторами
	var lines, trace []string
	nextVar, nextFile := 0, 0
	nOps := itembank.IntBetween(r, 4, 7)
	closed := false
	lastVar := ""
	for i := 0; i < nOps; i++ {
		last := i == nOps-1
		doClose := len(live) > 0 && !last && (r.IntN(3) == 0 || (!closed && i >= nOps-2))
		doDup := !doClose && len(live) > 0 && (r.IntN(4) == 0 || nextFile >= len(files))
		switch {
		case doClose:
			j := r.IntN(len(live))
			v := live[j]
			delete(open, v.fd)
			live = append(live[:j], live[j+1:]...)
			lines = append(lines, fmt.Sprintf("close(%s);", v.name))
			trace = append(trace, fmt.Sprintf("close(%s) освобождает %d", v.name, v.fd))
			closed = true
		case doDup:
			src := itembank.Pick(r, live)
			fd := lowest()
			open[fd] = true
			name := varNames[nextVar]
			nextVar++
			live = append(live, fdVar{name, fd})
			lines = append(lines, fmt.Sprintf("int %s = dup(%s);", name, src.name))
			trace = append(trace, fmt.Sprintf("%s = dup(%s) = %d", name, src.name, fd))
			lastVar = name
		default:
			fd := lowest()
			open[fd] = true
			name := varNames[nextVar]
			nextVar++
			live = append(live, fdVar{name, fd})
			lines = append(lines, fmt.Sprintf("int %s = open(\"%s\", O_RDONLY);", name, files[nextFile]))
			trace = append(trace, fmt.Sprintf("%s = %d", name, fd))
			nextFile++
			lastVar = name
		}
	}
	fds := make([]int, 0, len(open))
	for fd := range open {
		fds = append(fds, fd)
	}
	sort.Ints(fds)
	var question string
	var answer int
	if r.IntN(2) == 0 {
		question = fmt.Sprintf("Какое значение получит переменная `%s`?", lastVar)
		for _, v := range live {
			if v.name == lastVar {
				answer = v.fd
			}
		}
	} else {
		question = "Сколько файловых дескрипторов будет открыто у процесса после выполнения всех строк (включая стандартные потоки)?"
		answer = len(fds)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: "Процесс в Linux только что запущен: открыты только стандартные потоки (0 — stdin, 1 — stdout, 2 — stderr). " +
			"Он выполняет вызовы из листинга; все вызовы успешны. " + question,
		Code:   &itembank.CodeBlock{Lang: "c", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("open() и dup() возвращают наименьший свободный номер дескриптора, а close() освобождает номер для повторного использования. "+
			"Трассировка: %s. После всех операций открыты дескрипторы %s — всего %d. Типичная ошибка — начинать нумерацию с 0 или 1, забывая о стандартных "+
			"потоках, или считать, что освобождённый номер не переиспользуется.", strings.Join(trace, "; "), joinInts(fds, ", "), len(fds)),
	}, nil
}

// netTCPvsUDP — выбор транспортного протокола по сценарию.
type netTCPvsUDP struct{}

func (netTCPvsUDP) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.networking.tcp-vs-udp.v1", Spec: itembank.SpecCommon, Topic: "networking",
		Grade: itembank.GradeJunior, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.0, B: -0.7, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"tcp", "udp"},
	}
}

var udpScenarios = []string{
	"разрешение доменного имени `%[1]s.example.com` через DNS",
	"видеозвонок в реальном времени между сотрудниками: низкая задержка важнее доставки каждого кадра",
	"многопользовательская игра `%[1]s`, рассылающая координаты игроков %[2]d раз в секунду — устаревшие пакеты бесполезны",
	"датчики, отправляющие телеметрию каждые %[2]d0 мс, при этом потеря отдельных измерений допустима",
	"синхронизация часов серверов по NTP с `%[1]s.example.com`",
	"голосовая связь (VoIP) с кодеком, устойчивым к потере отдельных пакетов",
	"прямая трансляция эфира тысячам зрителей, где небольшие потери кадров допустимы",
}

var tcpScenarios = []string{
	"скачивание архива размером %[2]d0 МБ с сервера `%[1]s.example.com`: каждый байт должен прийти без потерь и в правильном порядке",
	"отправка электронного письма через SMTP-сервер `%[1]s.example.com`",
	"вызов REST API сервиса `%[1]s` для проведения платежа на %[2]d00 ₽",
	"интерактивная SSH-сессия администратора на сервере `%[1]s.example.com`",
	"загрузка веб-страницы интернет-магазина `%[1]s` по HTTPS",
	"репликация базы данных сервиса `%[1]s` между двумя дата-центрами",
	"передача файла отчёта размером %[2]d МБ по SFTP в банк",
}

func (f netTCPvsUDP) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE23)
	askUDP := r.IntN(2) == 0
	render := func(templates []string) []string {
		out := make([]string, 0, len(templates))
		for _, t := range templates {
			out = append(out, fmt.Sprintf(t, itembank.Ident(r, "service"), itembank.IntBetween(r, 2, 9)))
		}
		return out
	}
	udp := render(itembank.PickN(r, udpScenarios, itembank.IntBetween(r, 2, 3)))
	tcp := render(itembank.PickN(r, tcpScenarios, itembank.IntBetween(r, 2, 3)))
	correct, wrong, proto := udp, tcp, "UDP"
	if !askUDP {
		correct, wrong, proto = tcp, udp, "TCP"
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    fmt.Sprintf("Для каких из перечисленных сценариев целесообразнее выбрать %s в качестве транспортного протокола? Выберите все подходящие.", proto),
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: "UDP не устанавливает соединение и не повторяет потерянные датаграммы, поэтому даёт минимальную задержку: DNS, NTP, голос и видео " +
			"в реальном времени, игры и телеметрия — там опоздавший пакет уже не нужен. TCP гарантирует доставку и порядок байтов ценой задержек: файлы, " +
			"почта, платежи, SSH, HTTPS, репликация. Типичная ошибка — выбирать TCP «для надёжности» там, где повторная передача устаревших данных только вредит.",
	}, nil
}

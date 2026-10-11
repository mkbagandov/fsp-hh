package qa

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(contractStatus{})
	itembank.Register(retryPolicy{})
	itembank.Register(maskOutput{})
	itembank.Register(teardownOrder{})
}

// contractStatus — какой HTTP-статус вернёт сервер по контракту при заданном запросе,
// если проверки идут в фиксированном порядке до первой неудачи.
type contractStatus struct{}

func (contractStatus) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.automation_api.contract-status.v1", Spec: itembank.SpecQA, Topic: "automation_api",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.1, C: 0.25}, TimeLimit: 120 * time.Second, Tags: []string{"rest", "http", "contract"},
	}
}

type apiResource struct {
	path     string
	unique   string   // уникальное обязательное строковое поле
	existing []string // значения уникального поля у записей id 1..len
	fresh    []string // значения, которых в базе нет
	numField string
	lo, hi   int
	optField string
}

var apiResources = []apiResource{
	{"users", "email",
		[]string{"ivan@example.com", "olga@example.com", "petr@example.com", "anna@example.com", "sergey@example.com"},
		[]string{"alice@example.com", "bob@example.com", "carol@example.com", "dave@example.com", "erin@example.com"},
		"age", 18, 120, "nickname"},
	{"coupons", "code",
		[]string{"SPRING10", "SUMMER15", "WELCOME", "VIP2026", "BLACKFRI"},
		[]string{"AUTUMN20", "WINTER5", "HELLO", "NEWYEAR", "FLASH30"},
		"discount", 1, 90, "note"},
	{"projects", "slug",
		[]string{"billing-v2", "mobile-app", "data-lake", "crm-core", "web-portal"},
		[]string{"search-next", "payments-hub", "ml-platform", "auth-gateway", "docs-site"},
		"budget", 1, 1000000, "description"},
	{"tags", "name",
		[]string{"urgent", "backend", "frontend", "design", "infra"},
		[]string{"research", "legal", "hiring", "security", "mobile"},
		"weight", 0, 100, "color"},
}

var httpStatusText = map[int]string{
	200: "OK", 201: "Created", 400: "Bad Request", 401: "Unauthorized", 403: "Forbidden",
	404: "Not Found", 409: "Conflict", 422: "Unprocessable Entity",
}

func statusLabel(code int) string { return fmt.Sprintf("%d %s", code, httpStatusText[code]) }

func (f contractStatus) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A05)
	res := itembank.Pick(r, apiResources)
	isPut := r.IntN(2) == 0
	validationCode := itembank.Pick(r, []int{422, 422, 400})
	successCode := 201
	if isPut {
		successCode = 200
	}
	k := len(res.existing)
	// Основной сценарий: 0 — 401, 1 — 403, 2 — 404 (только PUT), 3 — валидация, 4 — 409, 5 — успех.
	scenarios := []int{0, 1, 3, 4, 5, 5}
	if isPut {
		scenarios = append(scenarios, 2, 2)
	}
	primary := itembank.Pick(r, scenarios)
	token := itembank.Pick(r, []string{"tok_admin", "tok_manager"})
	switch primary {
	case 0:
		token = itembank.Pick(r, []string{"", "tok_expired"})
	case 1:
		token = "tok_viewer"
	}
	id := itembank.IntBetween(r, 1, k)
	if primary == 2 {
		id = k + itembank.IntBetween(r, 1, 9)
	}
	// Ловушка: к основному нарушению иногда добавляется более позднее по порядку проверок.
	bodyBad := primary == 3 || (primary < 3 && r.IntN(3) == 0)
	conflict := primary == 4 || (primary < 3 && r.IntN(3) == 0)

	uniqueVal := itembank.Pick(r, res.fresh)
	conflictWith := 0
	if conflict {
		j := r.IntN(k)
		if isPut && j+1 == id {
			j = (j + 1) % k
		}
		uniqueVal = res.existing[j]
		conflictWith = j + 1
	}
	numVal := strconv.Itoa(itembank.IntBetween(r, res.lo, res.hi))
	violation := ""
	fields := []string{"\"" + res.unique + "\": \"" + uniqueVal + "\""}
	if bodyBad {
		switch r.IntN(4) {
		case 0:
			violation = fmt.Sprintf("обязательное поле `%s` отсутствует", res.numField)
		case 1:
			bad := res.lo - 1
			if r.IntN(2) == 0 {
				bad = res.hi + 1
			}
			violation = fmt.Sprintf("`%s` = %d вне диапазона %d..%d", res.numField, bad, res.lo, res.hi)
			fields = append(fields, fmt.Sprintf("\"%s\": %d", res.numField, bad))
		case 2:
			violation = fmt.Sprintf("`%s` передано строкой, а контракт требует integer", res.numField)
			fields = append(fields, "\""+res.numField+"\": \""+numVal+"\"")
		default:
			violation = fmt.Sprintf("обязательное поле `%s` пустое", res.unique)
			fields[0] = "\"" + res.unique + "\": \"\""
			fields = append(fields, "\""+res.numField+"\": "+numVal)
		}
	} else {
		fields = append(fields, "\""+res.numField+"\": "+numVal)
	}
	if r.IntN(2) == 0 {
		fields = append(fields, "\""+res.optField+"\": \""+itembank.Ident(r, "name_lat")+"\"")
	}
	body := "{" + strings.Join(fields, ", ") + "}"

	// Ответ вычисляется по порядку проверок независимо от сценария генерации.
	var checks []string
	var violated []int
	switch token {
	case "":
		checks = append(checks, "auth — заголовка Authorization нет → 401")
		violated = append(violated, 401)
	case "tok_expired":
		checks = append(checks, "auth — токен просрочен → 401")
		violated = append(violated, 401)
	default:
		checks = append(checks, "auth — токен валиден")
	}
	if token == "tok_viewer" {
		checks = append(checks, "role — viewer не входит в [admin, manager] → 403")
		violated = append(violated, 403)
	} else if token != "" && token != "tok_expired" {
		checks = append(checks, "role — "+strings.TrimPrefix(token, "tok_")+" может изменять")
	}
	if isPut {
		if id > k {
			checks = append(checks, fmt.Sprintf("exists — записи id=%d нет (есть 1..%d) → 404", id, k))
			violated = append(violated, 404)
		} else {
			checks = append(checks, fmt.Sprintf("exists — запись id=%d есть", id))
		}
	}
	if bodyBad {
		checks = append(checks, fmt.Sprintf("validation — %s → %d", violation, validationCode))
		violated = append(violated, validationCode)
	} else {
		checks = append(checks, "validation — тело соответствует схеме")
	}
	if conflict {
		checks = append(checks, fmt.Sprintf("uniqueness — %s=%s уже у записи id=%d → 409", res.unique, uniqueVal, conflictWith))
		violated = append(violated, 409)
	} else {
		checks = append(checks, fmt.Sprintf("uniqueness — %s свободен", res.unique))
	}
	status := successCode
	verdict := fmt.Sprintf("нарушений нет, запрос выполняется — %s", statusLabel(successCode))
	if len(violated) > 0 {
		status = violated[0]
		verdict = fmt.Sprintf("ответ определяет первое нарушение — %s", statusLabel(status))
	}
	// Дистракторы: сначала статусы остальных нарушений (ловушка «по самому заметному»), затем прочие.
	distractors := append([]int{}, violated[min(1, len(violated)):]...)
	pool := []int{successCode, 400, 401, 403, 404, 409, 422}
	var rest []int
	for _, c := range pool {
		dup := c == status
		for _, d := range distractors {
			if d == c {
				dup = true
			}
		}
		if !dup {
			rest = append(rest, c)
		}
	}
	for _, c := range itembank.PickN(r, rest, len(rest)) {
		if len(distractors) >= 3 {
			break
		}
		distractors = append(distractors, c)
	}
	labels := make([]string, 0, 3)
	for _, c := range distractors[:3] {
		labels = append(labels, statusLabel(c))
	}
	options, key := itembank.ShuffledOptions(r, statusLabel(status), labels...)

	method, path := "POST", "/api/v1/"+res.path
	order := "[auth, role, validation, uniqueness]"
	notFound := ""
	if isPut {
		method, path = "PUT", fmt.Sprintf("/api/v1/%s/{id}", res.path)
		order = "[auth, role, exists, validation, uniqueness]"
		notFound = "  not_found: 404            # записи {id} не существует\n"
	}
	contract := fmt.Sprintf("%s %s\nauth: Bearer-токен обязателен; изменять могут роли [admin, manager]\nchecks_order: %s\nresponses:\n  success: %d\n  unauthenticated: 401\n  forbidden: 403\n%s  validation_error: %d\n  conflict: 409             # %s уже занят другой записью\nbody:\n  %s: string, required, unique\n  %s: integer, required, %d..%d\n  %s: string, optional",
		method, path, order, successCode, notFound, validationCode, res.unique, res.unique, res.numField, res.lo, res.hi, res.optField)
	reqPath := "/api/v1/" + res.path
	if isPut {
		reqPath += "/" + strconv.Itoa(id)
	}
	reqLines := []string{method + " " + reqPath}
	if token != "" {
		reqLines = append(reqLines, "Authorization: Bearer "+token)
	}
	reqLines = append(reqLines, "Content-Type: application/json", "", body)
	request := strings.Join(reqLines, "\n")
	state := fmt.Sprintf("В базе есть записи id 1–%d со значениями `%s`: %s. Токены: `tok_admin` — роль admin, `tok_manager` — manager, `tok_viewer` — viewer, `tok_expired` — просрочен.",
		k, res.unique, strings.Join(res.existing, ", "))
	stems := []string{
		fmt.Sprintf("Автотест отправляет запрос к API, описанному контрактом ниже; сервер выполняет проверки строго в порядке `checks_order` и останавливается на первой неудачной. %s Запрос:\n\n```http\n%s\n```\n\nКакой HTTP-статус должен ожидать тест?", state, request),
		fmt.Sprintf("В контрактном тесте нужно задать ожидаемый статус ответа на запрос ниже. Сервер реализует контракт из блока кода и выполняет проверки в порядке `checks_order` до первой ошибки. %s\n\n```http\n%s\n```\n\nКакой статус вернёт сервер?", state, request),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "yaml", Source: contract},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: fmt.Sprintf("Проверки по порядку: %s. Итог: %s. "+
			"Типичная ошибка — выбирать код по самому заметному нарушению (например, 409 при занятом %s), хотя раньше срабатывает проверка аутентификации или прав, "+
			"или путать 401 (клиент не аутентифицирован) с 403 (аутентифицирован, но прав нет).",
			strings.Join(checks, "; "), verdict, res.unique),
	}, nil
}

// retryPolicy — сколько запросов отправит клиент, сколько прождёт и что вернёт при заданной политике повторов.
type retryPolicy struct{}

func (retryPolicy) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.automation_api.retry-policy.v1", Spec: itembank.SpecQA, Topic: "automation_api",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.8, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"http", "retry", "mock"},
	}
}

type stubResp struct {
	status, retryAfter int
}

func (f retryPolicy) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A06)
	maxAttempts := itembank.IntBetween(r, 3, 5)
	retryOn := itembank.Pick(r, [][]int{{429, 503}, {429, 502, 503, 504}, {500, 502, 503, 504}, {503, 504}})
	backoff := itembank.Pick(r, []string{"exponential", "exponential", "constant", "linear"})
	base := itembank.Pick(r, []int{100, 200, 250, 500})
	honor := r.IntN(2) == 0
	k := itembank.IntBetween(r, 1, maxAttempts)
	seq := make([]stubResp, maxAttempts)
	for i := range seq {
		switch {
		case i < k:
			st := itembank.Pick(r, []int{429, 429, 500, 502, 503, 503, 504})
			ra := 0
			if st == 429 && r.IntN(2) == 0 {
				ra = itembank.IntBetween(r, 1, 3)
			}
			seq[i] = stubResp{st, ra}
		case i == k:
			seq[i] = stubResp{itembank.Pick(r, []int{200, 200, 404, 400}), 0}
		default:
			seq[i] = stubResp{200, 0}
		}
	}
	isRetryable := func(st int) bool {
		for _, x := range retryOn {
			if x == st {
				return true
			}
		}
		return false
	}
	delay := func(n int) int {
		switch backoff {
		case "exponential":
			return base << (n - 1)
		case "linear":
			return base * n
		default:
			return base
		}
	}
	requests, wait, final := 0, 0, 0
	var trace []string
	for i := 1; i <= maxAttempts; i++ {
		resp := seq[i-1]
		requests++
		if !isRetryable(resp.status) {
			final = resp.status
			trace = append(trace, fmt.Sprintf("попытка %d: %d — статус не в retry_on, клиент возвращает его сразу", i, resp.status))
			break
		}
		if i == maxAttempts {
			final = resp.status
			trace = append(trace, fmt.Sprintf("попытка %d: %d — лимит попыток исчерпан, паузы нет, результат %d", i, resp.status, resp.status))
			break
		}
		d, src := delay(i), "backoff"
		if resp.status == 429 && resp.retryAfter > 0 && honor {
			d, src = resp.retryAfter*1000, "Retry-After"
		}
		wait += d
		trace = append(trace, fmt.Sprintf("попытка %d: %d — пауза %d мс (%s)", i, resp.status, d, src))
	}
	var seqParts []string
	for i, s := range seq {
		part := fmt.Sprintf("%d) %d", i+1, s.status)
		if s.retryAfter > 0 {
			part += fmt.Sprintf(" с заголовком `Retry-After: %d`", s.retryAfter)
		}
		seqParts = append(seqParts, part)
	}
	backoffComment := map[string]string{
		"exponential": "пауза перед попыткой n+1: base_ms * 2^(n-1)",
		"constant":    "пауза перед каждой следующей попыткой: base_ms",
		"linear":      "пауза перед попыткой n+1: base_ms * n",
	}[backoff]
	honorComment := "Retry-After игнорируется, всегда действует backoff"
	if honor {
		honorComment = "для 429 с заголовком Retry-After пауза = значение заголовка (секунды) вместо backoff"
	}
	var retryOnParts []string
	for _, c := range retryOn {
		retryOnParts = append(retryOnParts, strconv.Itoa(c))
	}
	policy := fmt.Sprintf("retry:\n  max_attempts: %d            # всего запросов, включая первый\n  retry_on: [%s]   # остальные статусы возвращаются вызывающему сразу\n  backoff: %s        # %s\n  base_ms: %d\n  honor_retry_after: %t     # %s",
		maxAttempts, strings.Join(retryOnParts, ", "), backoff, backoffComment, base, honor, honorComment)
	type question struct {
		text   string
		answer int
	}
	q := itembank.Pick(r, []question{
		{"Сколько HTTP-запросов всего отправит клиент?", requests},
		{"Сколько миллисекунд клиент суммарно проведёт в паузах между попытками?", wait},
		{"Какой HTTP-статус клиент вернёт тесту в итоге?", final},
	})
	stems := []string{
		fmt.Sprintf("HTTP-клиент автотестов настроен политикой повторов из блока кода. Заглушка сервера отвечает на последовательные запросы так: %s. Пауза делается только перед следующей попыткой. %s", strings.Join(seqParts, "; "), q.text),
		fmt.Sprintf("В интеграционном тесте мок-сервер возвращает на подряд идущие запросы: %s. Клиент работает по политике повторов ниже (пауза — только между попытками, после последней её нет). %s", strings.Join(seqParts, "; "), q.text),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "yaml", Source: policy},
		Answer: itembank.Answer{Number: float64(q.answer)},
		Explanation: fmt.Sprintf("Трассировка: %s. Итого запросов %d, суммарная пауза %d мс, результат %d. "+
			"Типичные ошибки — повторять статусы вне retry_on (в том числе 4xx вроде 404) при оставшихся попытках, добавлять паузу после последней попытки "+
			"или складывать Retry-After с backoff вместо замены.",
			strings.Join(trace, "; "), requests, wait, final),
	}, nil
}

// maskOutput — что выведет скрипт маскирования персональных данных для тестовой выборки.
type maskOutput struct{}

func (maskOutput) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.test_data.mask-output.v1", Spec: itembank.SpecQA, Topic: "test_data",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.4, B: -0.6, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"python", "pii", "anonymization"},
	}
}

var (
	maskLocals  = []string{"ivan.petrov", "o.smirnova", "dmitry_k", "anna.lee", "s.volkov", "maria88", "petr.ivanov", "k.orlova", "nikita.b", "elena.s"}
	maskDomains = []string{"corp.example", "mail.test", "shop.local", "example.org", "team.dev"}
)

func randDigits(r *rand.Rand, n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString(strconv.Itoa(r.IntN(10)))
	}
	return sb.String()
}

func (f maskOutput) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A07)
	var code, out, why string
	switch r.IntN(3) {
	case 0:
		pan := itembank.Pick(r, []string{"4", "5", "2"}) + randDigits(r, 15)
		keepFirst := itembank.Pick(r, []int{4, 6})
		keepLast := itembank.Pick(r, []int{4, 2})
		mask := itembank.Pick(r, []string{"*", "#", "X"})
		hidden := len(pan) - keepFirst - keepLast
		masked := pan[:keepFirst] + strings.Repeat(mask, hidden) + pan[len(pan)-keepLast:]
		code = fmt.Sprintf("def mask_pan(pan: str) -> str:\n    hidden = len(pan) - %d - %d\n    return pan[:%d] + \"%s\" * hidden + pan[-%d:]\n\n", keepFirst, keepLast, keepFirst, mask, keepLast)
		if r.IntN(2) == 0 {
			code += fmt.Sprintf("masked = mask_pan(\"%s\")\nprint(\" \".join(masked[i:i + 4] for i in range(0, len(masked), 4)))", pan)
			var groups []string
			for i := 0; i < len(masked); i += 4 {
				groups = append(groups, masked[i:i+4])
			}
			out = strings.Join(groups, " ")
			why = fmt.Sprintf("len(pan) = 16, скрывается 16 − %d − %d = %d символов: `pan[:%d]` — первые %d, `pan[-%d:]` — последние %d, между ними %d знаков «%s». "+
				"Группировка по четыре применяется уже к замаскированной строке `%s`, поэтому границы групп не совпадают с границами маски.",
				keepFirst, keepLast, hidden, keepFirst, keepFirst, keepLast, keepLast, hidden, mask, masked)
		} else {
			code += fmt.Sprintf("print(mask_pan(\"%s\"))", pan)
			out = masked
			why = fmt.Sprintf("len(pan) = 16, скрывается 16 − %d − %d = %d символов: `pan[:%d]` — первые %d цифры, `pan[-%d:]` — последние %d, между ними ровно %d знаков «%s». "+
				"Типичная ошибка — ошибиться в числе скрытых символов или в длине хвоста при отрицательном срезе.",
				keepFirst, keepLast, hidden, keepFirst, keepFirst, keepLast, keepLast, hidden, mask)
		}
	case 1:
		local := itembank.Pick(r, maskLocals)
		domain := itembank.Pick(r, maskDomains)
		email := local + "@" + domain
		if r.IntN(2) == 0 {
			k := itembank.IntBetween(r, 1, 3)
			code = fmt.Sprintf("def mask_email(email: str) -> str:\n    local, domain = email.split(\"@\")\n    return local[:%d] + \"*\" * (len(local) - %d) + \"@\" + domain\n\nprint(mask_email(\"%s\"))", k, k, email)
			out = local[:k] + strings.Repeat("*", len(local)-k) + "@" + domain
			why = fmt.Sprintf("local = «%s» (%d символов): остаются первые %d, далее %d − %d = %d звёздочек; домен «%s» не маскируется. "+
				"Типичная ошибка — ставить звёздочки по длине всего адреса или маскировать и домен.", local, len(local), k, len(local), k, len(local)-k, domain)
		} else {
			code = fmt.Sprintf("def mask_email(email: str) -> str:\n    local, domain = email.split(\"@\")\n    return local[0] + \"*\" * (len(local) - 2) + local[-1] + \"@\" + domain\n\nprint(mask_email(\"%s\"))", email)
			out = local[:1] + strings.Repeat("*", len(local)-2) + local[len(local)-1:] + "@" + domain
			why = fmt.Sprintf("local = «%s» (%d символов): первый символ «%s» и последний «%s» остаются, между ними %d − 2 = %d звёздочек; домен не меняется. "+
				"Типичная ошибка — забыть, что `local[-1]` уже учтён, и поставить на одну звёздочку больше.", local, len(local), local[:1], local[len(local)-1:], len(local), len(local)-2)
		}
	default:
		d := "7" + itembank.Pick(r, []string{"916", "903", "921", "985", "912", "926"}) + randDigits(r, 7)
		phone := itembank.Pick(r, []string{
			fmt.Sprintf("+7 %s %s-%s-%s", d[1:4], d[4:7], d[7:9], d[9:11]),
			fmt.Sprintf("+7 (%s) %s-%s-%s", d[1:4], d[4:7], d[7:9], d[9:11]),
			fmt.Sprintf("+7%s", d[1:]),
		})
		head := "def mask_phone(phone: str) -> str:\n    d = \"\".join(ch for ch in phone if ch.isdigit())\n"
		if r.IntN(2) == 0 {
			code = head + fmt.Sprintf("    return \"+\" + d[0] + \" (\" + d[1:4] + \") ***-**-\" + d[-2:]\n\nprint(mask_phone(\"%s\"))", phone)
			out = "+" + d[:1] + " (" + d[1:4] + ") ***-**-" + d[9:11]
			why = fmt.Sprintf("После фильтра `isdigit` остаются только цифры: «%s». `d[0]` = %s, `d[1:4]` = %s (код), `d[-2:]` = %s; остальное заменено маской. "+
				"Типичная ошибка — брать срезы от исходной строки с пробелами и скобками, а не от очищенной.", d, d[:1], d[1:4], d[9:11])
		} else {
			code = head + fmt.Sprintf("    return \"+\" + d[0] + \" (***) \" + d[4:7] + \"-**-\" + d[-2:]\n\nprint(mask_phone(\"%s\"))", phone)
			out = "+" + d[:1] + " (***) " + d[4:7] + "-**-" + d[9:11]
			why = fmt.Sprintf("После фильтра `isdigit` остаются только цифры: «%s». Код скрыт, `d[4:7]` = %s — три цифры после кода, `d[-2:]` = %s. "+
				"Типичная ошибка — считать, что `d[4:7]` берёт код оператора, или делать срезы по исходной строке с разделителями.", d, d[4:7], d[9:11])
		}
	}
	stems := []string{
		"Скрипт маскирует персональные данные при подготовке тестовой выборки из дампа продакшена. Что он выведет? Введите строку точно, с учётом регистра и пробелов.",
		"Перед загрузкой в тестовую базу данные анонимизируют функцией ниже. Какую строку напечатает `print`?",
		"QA-инженер проверяет функцию маскирования тестовых данных. Что окажется в стандартном выводе после запуска скрипта?",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        itembank.Pick(r, stems),
		Code:        &itembank.CodeBlock{Lang: "python", Source: code},
		Answer:      itembank.Answer{Text: out},
		Explanation: fmt.Sprintf("Вывод: `%s`. %s", out, why),
	}, nil
}

// teardownOrder — в каком порядке удалять (или вставлять) тестовые данные при цепочке внешних ключей.
type teardownOrder struct{}

func (teardownOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "qa.test_data.teardown-order.v1", Spec: itembank.SpecQA, Topic: "test_data",
		Grade: itembank.GradeSenior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.4, B: 0.7, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"sql", "fixtures", "foreign-keys"},
	}
}

type tdTable struct {
	name, singular string
}

// Цепочки «лист → … → корень»: каждая таблица ссылается на следующую.
var tdChains = [][]tdTable{
	{{"order_items", "order_item"}, {"orders", "order"}, {"customers", "customer"}, {"cities", "city"}, {"regions", "region"}, {"countries", "country"}},
	{{"comments", "comment"}, {"tasks", "task"}, {"projects", "project"}, {"users", "user"}, {"tenants", "tenant"}, {"plans", "plan"}},
	{{"payslip_lines", "payslip_line"}, {"payslips", "payslip"}, {"employees", "employee"}, {"departments", "department"}, {"companies", "company"}, {"holdings", "holding"}},
	{{"review_votes", "review_vote"}, {"reviews", "review"}, {"products", "product"}, {"brands", "brand"}, {"suppliers", "supplier"}, {"countries", "country"}},
	{{"ticket_events", "ticket_event"}, {"tickets", "ticket"}, {"queues", "queue"}, {"teams", "team"}, {"departments", "department"}, {"orgs", "org"}},
	{{"lesson_marks", "lesson_mark"}, {"lessons", "lesson"}, {"courses", "course"}, {"teachers", "teacher"}, {"schools", "school"}, {"districts", "district"}},
}

func (f teardownOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x9A08)
	chain := itembank.Pick(r, tdChains)[:itembank.IntBetween(r, 5, 6)]
	n := len(chain)
	var fks []string
	for i := 0; i+1 < n; i++ {
		child, parent := chain[i], chain[i+1]
		fks = append(fks, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT fk_%s_%s\n    FOREIGN KEY (%s_id) REFERENCES %s (id);",
			child.name, child.name, parent.singular, parent.singular, parent.name))
	}
	fks = itembank.Shuffle(r, fks)
	names := make([]string, n)
	for i, t := range chain {
		names[i] = t.name
	}
	isDelete := r.IntN(2) == 0
	ordered := append([]string{}, names...)
	if !isDelete {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			ordered[i], ordered[j] = ordered[j], ordered[i]
		}
	}
	options, order := itembank.OrderingOptions(r, ordered)
	listed := strings.Join(itembank.Shuffle(r, names), ", ")
	var stems []string
	if isDelete {
		stems = []string{
			fmt.Sprintf("Фикстура интеграционных тестов создала строки в таблицах %s. Внешние ключи заданы в блоке кода, каскадного удаления (ON DELETE CASCADE) нет. Расставьте таблицы в порядке, в котором teardown должен выполнять `DELETE`, чтобы ни одна команда не нарушила внешний ключ.", listed),
			fmt.Sprintf("После прогона тестов нужно очистить тестовые данные из таблиц %s обычными `DELETE` (внешние ключи — ниже, каскадов нет). В каком порядке удалять строки?", listed),
		}
	} else {
		stems = []string{
			fmt.Sprintf("Сидер тестовых данных вставляет по одной строке в каждую из таблиц %s. Внешние ключи заданы в блоке кода (все столбцы-ссылки NOT NULL). Расставьте таблицы в порядке, в котором нужно выполнять `INSERT`, чтобы каждая вставка ссылалась на уже существующую строку.", listed),
			fmt.Sprintf("Фикстура наполняет тестовую базу: таблицы %s связаны внешними ключами из блока кода (NOT NULL, проверка не отложена). В каком порядке вставлять строки?", listed),
		}
	}
	action := "При `DELETE` первыми удаляют ссылающиеся (дочерние) строки, корень цепочки — последним"
	if !isDelete {
		action = "При `INSERT` сначала создают корень цепочки, на который никто не ссылается, и идут к листу"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "sql", Source: strings.Join(fks, "\n")},
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: fmt.Sprintf("Цепочка ссылок (→ = «ссылается на»): %s. %s. Верный порядок: %s. "+
			"Типичная ошибка — перепутать направление (удалить родителя первым или вставить лист до родителя) либо ориентироваться на порядок объявления constraint в файле.",
			strings.Join(names, " → "), action, strings.Join(ordered, ", ")),
	}, nil
}

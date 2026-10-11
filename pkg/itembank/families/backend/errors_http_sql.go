package backend

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(wrapIsChain{})
	itembank.Register(panicDeferTrace{})
	itembank.Register(middlewareOrder{})
	itembank.Register(handlerStatus{})
	itembank.Register(rankTies{})
}

// wrapIsChain — что вернёт errors.Is по цепочке обёрток с %w и %v.
type wrapIsChain struct{}

func (wrapIsChain) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.errors_logging.wrap-is-chain.v1", Spec: itembank.SpecBackend, Topic: "errors_logging",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.5, B: -0.5, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"go", "errors", "wrapping"},
	}
}

type wrapSentinel struct{ name, msg string }

var wrapSentinels = []wrapSentinel{
	{"ErrNotFound", "not found"}, {"ErrTimeout", "timeout"}, {"ErrConflict", "conflict"},
	{"ErrDenied", "access denied"}, {"ErrInvalid", "invalid input"},
}

func (f wrapIsChain) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE601)
	sentinels := itembank.PickN(r, wrapSentinels, 2)
	bottom := r.IntN(2)
	layers := itembank.IntBetween(r, 2, 3)
	funcs := itembank.PickN(r, []string{"fetch", "load", "query", "save", "sync", "parse", "handle", "process", "store"}, layers)
	verbs := make([]string, layers)
	for i := range verbs {
		verbs[i] = "%w"
	}
	broken := -1
	if r.IntN(2) == 0 {
		broken = r.IntN(layers)
		verbs[broken] = itembank.Pick(r, []string{"%v", "%s"})
	}
	width := len(sentinels[0].name)
	if len(sentinels[1].name) > width {
		width = len(sentinels[1].name)
	}
	lines := []string{"package main", "", "import (", "\t\"errors\"", "\t\"fmt\"", ")", "", "var ("}
	for _, s := range sentinels {
		lines = append(lines, fmt.Sprintf("\t%-*s = errors.New(%q)", width, s.name, s.msg))
	}
	lines = append(lines, ")", "")
	msg := sentinels[bottom].msg
	var chainDesc []string
	for i, fn := range funcs {
		arg := sentinels[bottom].name
		if i > 0 {
			arg = funcs[i-1] + "()"
		}
		lines = append(lines, fmt.Sprintf("func %s() error { return fmt.Errorf(\"%s: %s\", %s) }", fn, fn, verbs[i], arg))
		msg = fn + ": " + msg
		chainDesc = append(chainDesc, fmt.Sprintf("`%s` → `%s`", fn, verbs[i]))
	}
	lines = append(lines, "", "func main() {", fmt.Sprintf("\terr := %s()", funcs[layers-1]),
		fmt.Sprintf("\tfmt.Println(errors.Is(err, %s), errors.Is(err, %s))", sentinels[0].name, sentinels[1].name),
		"\tfmt.Println(err)", "}")
	intact := broken < 0
	is := [2]bool{intact && bottom == 0, intact && bottom == 1}
	chainNote := "все звенья используют `%w`, поэтому цепочка Unwrap доходит до " + sentinels[bottom].name
	if !intact {
		chainNote = fmt.Sprintf("звено `%s` использует `%s`, которое подставляет только текст и обрывает цепочку Unwrap — ниже него errors.Is не заглянет", funcs[broken], verbs[broken])
	}
	stems := []string{
		"Что выведет программа? Укажите обе строки вывода.",
		"Какие две строки напечатает программа (булевы значения — как их печатает fmt: `true`/`false`)?",
		"Что окажется в stdout после запуска программы? Приведите вывод построчно.",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "go", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Text: fmt.Sprintf("%v %v\n%s", is[0], is[1], msg)},
		Explanation: fmt.Sprintf("`%%w` сохраняет обёрнутую ошибку для errors.Unwrap, а `%%v` и `%%s` подставляют только её текст. Цепочка снизу вверх: %s; %s. "+
			"Сообщение собирается одинаково при любом глаголе: «%s». Итог: errors.Is(err, %s) = %v, errors.Is(err, %s) = %v (%s в цепочке нет вовсе). "+
			"Типичная ошибка — судить по тексту: подстрока «%s» в сообщении есть, но errors.Is сравнивает звенья цепочки, а не строку.",
			strings.Join(chainDesc, ", "), chainNote, msg, sentinels[0].name, is[0], sentinels[1].name, is[1], sentinels[1-bottom].name, sentinels[bottom].msg),
	}, nil
}

// panicDeferTrace — порядок вывода при panic, defer и recover в вызываемой функции или в main.
type panicDeferTrace struct{}

func (panicDeferTrace) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.errors_logging.panic-defer-trace.v1", Spec: itembank.SpecBackend, Topic: "errors_logging",
		Grade: itembank.GradeSenior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.5, B: 0.6, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"go", "panic", "recover", "defer"},
	}
}

const recoverDefer = "\x00recover"

func (f panicDeferTrace) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE602)
	fn := itembank.Pick(r, []string{"process", "handle", "run", "execute", "apply"})
	msg := itembank.Pick(r, []string{"too big", "bad input", "overflow", "no space", "limit exceeded"})
	threshold := itembank.IntBetween(r, 2, 6)
	arg := threshold - itembank.IntBetween(r, 0, 2)
	if r.IntN(3) > 0 {
		arg = threshold + itembank.IntBetween(r, 1, 3)
	}
	recoverInMain := r.IntN(3) == 0
	calleeDefers := itembank.PickN(r, []string{"cleanup", "unlock", "close", "flush"}, itembank.IntBetween(r, 1, 2))
	mainDefers := []string{itembank.Pick(r, []string{"exit", "shutdown", "bye"})}
	insertAt := func(xs []string, item string) []string {
		i := r.IntN(len(xs) + 1)
		out := append([]string{}, xs[:i]...)
		out = append(out, item)
		return append(out, xs[i:]...)
	}
	if recoverInMain {
		mainDefers = insertAt(mainDefers, recoverDefer)
	} else {
		calleeDefers = insertAt(calleeDefers, recoverDefer)
	}
	deferLines := func(scope string, items []string) []string {
		var ls []string
		for _, it := range items {
			if it == recoverDefer {
				ls = append(ls, "\tdefer func() {", "\t\tif r := recover(); r != nil {", "\t\t\tfmt.Println(\"recovered:\", r)", "\t\t}", "\t}()")
				continue
			}
			ls = append(ls, fmt.Sprintf("\tdefer fmt.Println(\"%s: %s\")", scope, it))
		}
		return ls
	}
	lines := []string{"package main", "", "import \"fmt\"", "", fmt.Sprintf("func %s(n int) {", fn)}
	lines = append(lines, deferLines(fn, calleeDefers)...)
	lines = append(lines, fmt.Sprintf("\tfmt.Println(\"%s: start\")", fn), fmt.Sprintf("\tif n > %d {", threshold), fmt.Sprintf("\t\tpanic(%q)", msg), "\t}",
		fmt.Sprintf("\tfmt.Println(\"%s: done\")", fn), "}", "", "func main() {")
	lines = append(lines, deferLines("main", mainDefers)...)
	lines = append(lines, fmt.Sprintf("\t%s(%d)", fn, arg), "\tfmt.Println(\"main: after\")", "}")

	// Симуляция: panic запускает defer текущего кадра (LIFO); recover гасит панику только в отложенной функции своего кадра.
	panicking := arg > threshold
	out := []string{fn + ": start"}
	if !panicking {
		out = append(out, fn+": done")
	}
	runDefers := func(scope string, items []string) {
		for i := len(items) - 1; i >= 0; i-- {
			if items[i] == recoverDefer {
				if panicking {
					out = append(out, "recovered: "+msg)
					panicking = false
				}
				continue
			}
			out = append(out, scope+": "+items[i])
		}
	}
	runDefers(fn, calleeDefers)
	if !panicking {
		out = append(out, "main: after")
	}
	runDefers("main", mainDefers)
	if panicking {
		return itembank.Instance{}, fmt.Errorf("seed %d: panic is never recovered", seed)
	}
	var path string
	switch {
	case arg <= threshold:
		path = fmt.Sprintf("%d ≤ %d, паники нет: функция печатает start и done, её defer выполняются в обратном порядке, recover() возвращает nil и ничего не печатает, main продолжает работу", arg, threshold)
	case recoverInMain:
		path = fmt.Sprintf("%d > %d — паника. Отложенные вызовы `%s` выполняются в обратном порядке (recover среди них нет), затем паника поднимается в main: строка «main: after» пропускается, выполняются только defer main, и recover в одном из них останавливает панику", arg, threshold, fn)
	default:
		path = fmt.Sprintf("%d > %d — паника, «%s: done» не печатается. Defer функции `%s` выполняются в обратном порядке регистрации, recover в отложенной функции гасит панику (остальные defer всё равно выполняются), после чего `%s` завершается штатно и main продолжает работу", arg, threshold, fn, fn, fn)
	}
	stems := []string{
		"Что выведет программа? Перечислите строки по порядку.",
		fmt.Sprintf("Что напечатает программа при вызове `%s(%d)`? Укажите все строки вывода, по одной на строку.", fn, arg),
		"Какие строки появятся в stdout и в каком порядке? Приведите вывод построчно.",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "go", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Text: strings.Join(out, "\n")},
		Explanation: "panic запускает отложенные вызовы текущего кадра в порядке LIFO, а recover() действует только внутри отложенной функции того кадра, где он вызван; после recover функция возвращается к вызывающему штатно. " +
			"Здесь " + path + ". Порядок вывода: " + strings.Join(out, " → ") + ". " +
			"Типичная ошибка — продолжать выполнение со строки после panic или печатать «main: after», когда recover стоит в main.",
	}, nil
}

// middlewareOrder — порядок строк вывода цепочки middleware вокруг обработчика.
type middlewareOrder struct{}

func (middlewareOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.http_rest.middleware-order.v1", Spec: itembank.SpecBackend, Topic: "http_rest",
		Grade: itembank.GradeMiddle, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.6, B: 0.1, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"go", "http", "middleware"},
	}
}

func (f middlewareOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE603)
	names := itembank.PickN(r, []string{"auth", "logging", "metrics", "tracing", "cors", "ratelimit", "recover", "timeout"}, 3)
	nBoth := itembank.IntBetween(r, 1, 2)
	type mw struct {
		name          string
		before, after bool
	}
	mws := make([]mw, len(names))
	for i, n := range names {
		switch {
		case i < nBoth:
			mws[i] = mw{n, true, true}
		case r.IntN(2) == 0:
			mws[i] = mw{n, true, false}
		default:
			mws[i] = mw{n, false, true}
		}
	}
	mws = itembank.Shuffle(r, mws)
	reverse := r.IntN(2) == 0
	req := itembank.Pick(r, []string{"GET /orders", "POST /payments", "GET /users/7", "DELETE /sessions/3", "PUT /items/42"})
	loop := "\tfor _, m := range ms {\n\t\th = m(h)\n\t}"
	if reverse {
		loop = "\tfor i := len(ms) - 1; i >= 0; i-- {\n\t\th = ms[i](h)\n\t}"
	}
	calls := make([]string, len(mws))
	for i, m := range mws {
		calls[i] = fmt.Sprintf("mw(%q, %v, %v)", m.name, m.before, m.after)
	}
	code := fmt.Sprintf(`type Handler func(req string)

func chain(h Handler, ms ...func(Handler) Handler) Handler {
%s
	return h
}

func mw(name string, before, after bool) func(Handler) Handler {
	return func(next Handler) Handler {
		return func(req string) {
			if before {
				fmt.Println(name, "before")
			}
			next(req)
			if after {
				fmt.Println(name, "after")
			}
		}
	}
}

func main() {
	h := chain(func(req string) { fmt.Println("handler", req) },
		%s)
	h(%q)
}`, loop, strings.Join(calls, ", "), req)

	// Снаружи внутрь: при обратном цикле внешним становится первый в списке, при прямом — последний.
	outer := append([]mw{}, mws...)
	if !reverse {
		for i, j := 0, len(outer)-1; i < j; i, j = i+1, j-1 {
			outer[i], outer[j] = outer[j], outer[i]
		}
	}
	var lines, nesting []string
	for _, m := range outer {
		nesting = append(nesting, m.name)
		if m.before {
			lines = append(lines, m.name+" before")
		}
	}
	lines = append(lines, "handler "+req)
	for i := len(outer) - 1; i >= 0; i-- {
		if outer[i].after {
			lines = append(lines, outer[i].name+" after")
		}
	}
	options, order := itembank.OrderingOptions(r, lines)
	direction := "цикл идёт по списку слева направо, каждый следующий middleware оборачивает уже собранную цепочку, поэтому внешним становится последний в списке"
	if reverse {
		direction = "цикл идёт с конца списка, поэтому первый middleware в списке оборачивает все остальные и оказывается внешним"
	}
	stems := []string{
		"Программа собирает цепочку middleware функцией `chain` и вызывает обработчик. Расставьте строки вывода в том порядке, в котором они будут напечатаны.",
		"В каком порядке появятся строки в stdout при выполнении `main`? Упорядочьте их от первой к последней.",
		"Функция `chain` оборачивает обработчик middleware из списка. Упорядочьте напечатанные строки от первой к последней.",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "go", Source: code},
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: fmt.Sprintf("В `chain` %s. Вложенность снаружи внутрь: %s → handler. Код до `next(req)` выполняется на входе (снаружи внутрь), код после `next` — на выходе (изнутри наружу), "+
			"поэтому порядок: %s. Типичная ошибка — считать порядок в списке порядком выполнения «after» или забывать, что внешний middleware завершает работу последним.",
			direction, strings.Join(nesting, " → "), strings.Join(lines, " → ")),
	}, nil
}

// handlerStatus — какой код статуса вернёт обработчик при заданном запросе.
type handlerStatus struct{}

func (handlerStatus) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.http_rest.handler-status.v1", Spec: itembank.SpecBackend, Topic: "http_rest",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: -0.6, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"go", "http", "rest"},
	}
}

type statusCheck struct {
	key, cond, status, desc string
	code                    int
}

var statusChecks = []statusCheck{
	{"method", "r.Method != http.MethodPatch", "StatusMethodNotAllowed", "метод не PATCH", 405},
	{"auth", `r.Header.Get("Authorization") == ""`, "StatusUnauthorized", "нет заголовка Authorization", 401},
	{"role", `r.Header.Get("X-Role") != "admin"`, "StatusForbidden", "X-Role не равен admin", 403},
	{"id", "err != nil", "StatusBadRequest", "id не разбирается как число", 400},
	{"found", "!found", "StatusNotFound", "записи с таким id нет", 404},
	{"etag", `r.Header.Get("If-Match") != "" && r.Header.Get("If-Match") != etag`, "StatusPreconditionFailed", "If-Match задан и не совпадает с ETag записи", 412},
	{"qty", "body.Qty < 0", "StatusUnprocessableEntity", "qty отрицательный", 422},
}

func (f handlerStatus) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE605)
	var found statusCheck
	var rest []statusCheck
	for _, c := range statusChecks {
		if c.key == "found" {
			found = c
		} else {
			rest = append(rest, c)
		}
	}
	chosen := itembank.Shuffle(r, append(itembank.PickN(r, rest, 4), found))
	fi, ei := -1, -1
	for i, c := range chosen {
		switch c.key {
		case "found":
			fi = i
		case "etag":
			ei = i
		}
	}
	if ei >= 0 && ei < fi {
		chosen[fi], chosen[ei] = chosen[ei], chosen[fi]
	}
	has := func(key string) bool {
		for _, c := range chosen {
			if c.key == key {
				return true
			}
		}
		return false
	}
	ids := itembank.PickN(r, []int{3, 7, 12, 42, 58, 73, 91}, 2)
	versions := itembank.PickN(r, []int{1, 2, 3, 4, 5, 6}, 2)
	etags := map[int]string{ids[0]: fmt.Sprintf("\"v%d\"", versions[0]), ids[1]: fmt.Sprintf("\"v%d\"", versions[1])}

	// Запрос: каждое свойство может нарушать «своё» условие независимо от остальных.
	method := "PATCH"
	if r.IntN(4) == 0 {
		method = itembank.Pick(r, []string{"PUT", "POST", "GET"})
	}
	authHdr := r.IntN(4) > 0
	role := "admin"
	switch r.IntN(5) {
	case 0:
		role = ""
	case 1:
		role = itembank.Pick(r, []string{"editor", "viewer"})
	}
	var idStr string
	switch k := r.IntN(10); {
	case k < 5:
		idStr = strconv.Itoa(itembank.Pick(r, ids))
	case k < 8:
		for {
			n := itembank.Pick(r, []int{3, 7, 12, 42, 58, 73, 91})
			if n != ids[0] && n != ids[1] {
				idStr = strconv.Itoa(n)
				break
			}
		}
	default:
		idStr = itembank.Pick(r, []string{"abc", "last", "42a"})
	}
	id, err := strconv.Atoi(idStr)
	etag, ok := etags[id]
	ifMatch := ""
	switch k := r.IntN(10); {
	case k < 4:
	case k < 7 && ok:
		ifMatch = etag
	default:
		for {
			v := itembank.IntBetween(r, 1, 6)
			if s := fmt.Sprintf("\"v%d\"", v); s != etag {
				ifMatch = s
				break
			}
		}
	}
	qty := itembank.IntBetween(r, 1, 20)
	if r.IntN(3) == 0 {
		qty = -itembank.IntBetween(r, 1, 9)
	}
	fails := map[string]bool{
		"method": method != "PATCH", "auth": !authHdr, "role": role != "admin", "id": err != nil,
		"found": !ok, "etag": ifMatch != "" && ifMatch != etag, "qty": qty < 0,
	}
	answer, result := 200, "ни одно условие не сработало, обработчик возвращает http.StatusOK = 200"
	var trace []string
	for _, c := range chosen {
		if fails[c.key] {
			if answer == 200 {
				answer = c.code
				result = fmt.Sprintf("первое сработавшее условие — «%s», ответ http.%s = %d", c.desc, c.status, c.code)
			}
			trace = append(trace, c.desc+": да")
		} else {
			trace = append(trace, c.desc+": нет")
		}
	}

	idDecl := "\tid, err := strconv.Atoi(r.PathValue(\"id\"))"
	if !has("id") {
		idDecl = "\tid, _ := strconv.Atoi(r.PathValue(\"id\"))"
	}
	etagDecl := "\tetag, found := etags[id]"
	if !has("etag") {
		etagDecl = "\t_, found := etags[id]"
	}
	lines := []string{fmt.Sprintf("var etags = map[int]string{%d: `%s`, %d: `%s`}", ids[0], etags[ids[0]], ids[1], etags[ids[1]]), "",
		"func status(r *http.Request) int {", idDecl, etagDecl}
	if has("qty") {
		lines = append(lines, "\tvar body struct{ Qty int }", "\t_ = json.NewDecoder(r.Body).Decode(&body)")
	}
	lines = append(lines, "\tswitch {")
	for _, c := range chosen {
		lines = append(lines, "\tcase "+c.cond+":", "\t\treturn http."+c.status)
	}
	lines = append(lines, "\t}", "\treturn http.StatusOK", "}")

	reqLines := []string{fmt.Sprintf("%s /items/%s HTTP/1.1", method, idStr)}
	if authHdr {
		reqLines = append(reqLines, "Authorization: Bearer "+itembank.Pick(r, []string{"eyJhbGciOiJIUzI1NiJ9.e30.x", "tok_7f3a9c", "s3cr3t-token"}))
	}
	if role != "" {
		reqLines = append(reqLines, "X-Role: "+role)
	}
	if ifMatch != "" {
		reqLines = append(reqLines, "If-Match: "+ifMatch)
	}
	reqLines = append(reqLines, "Content-Type: application/json", "", fmt.Sprintf(`{"qty": %d}`, qty))
	request := "```http\n" + strings.Join(reqLines, "\n") + "\n```"
	stems := []string{
		fmt.Sprintf("Функция `status` обслуживает маршрут `/items/{id}` для всех методов (`r.PathValue(\"id\")` возвращает сегмент пути). На сервер приходит запрос:\n\n%s\n\nКакой код статуса HTTP вернёт `status`? Ответ — число.", request),
		fmt.Sprintf("Запрос ниже доходит до функции `status`, зарегистрированной на `/items/{id}` без ограничения по методу:\n\n%s\n\nУкажите числовой код статуса, который вернёт функция.", request),
	}
	zeroNote := ""
	if err != nil && !has("id") {
		zeroNote = " Нечисловой id при отброшенной ошибке даёт id = 0, которого в etags нет."
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "go", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Условия `switch` проверяются сверху вниз, срабатывает первое истинное: %s. Итог: %s.%s "+
			"Типичная ошибка — выбирать «самую серьёзную» проблему запроса вместо первой по порядку в коде или считать отсутствующий If-Match несовпадением.",
			strings.Join(trace, "; "), result, zeroNote),
	}, nil
}

// rankTies — результат RANK / DENSE_RANK / ROW_NUMBER при равных значениях.
type rankTies struct{}

func (rankTies) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.sql.rank-ties.v1", Spec: itembank.SpecBackend, Topic: "sql",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.5, B: 0.3, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"sql", "window", "postgres"},
	}
}

func (f rankTies) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE604)
	table := itembank.Pick(r, []string{"results", "scores", "leaderboard", "attempts", "submissions"})
	type measure struct {
		col  string
		desc bool
		pool []int
	}
	m := itembank.Pick(r, []measure{
		{"score", true, []int{60, 70, 80, 90, 100}},
		{"points", true, []int{12, 15, 18, 21, 24}},
		{"duration", false, []int{35, 42, 50, 58, 65}},
		{"errors", false, []int{0, 1, 2, 3, 5}},
	})
	names := itembank.Idents(r, "name_lat", itembank.IntBetween(r, 6, 7))
	type row struct {
		name string
		val  int
	}
	rows := make([]row, len(names))
	for {
		vals := itembank.PickN(r, m.pool, 4)
		distinct := map[int]bool{}
		for i, n := range names {
			rows[i] = row{n, itembank.Pick(r, vals)}
			distinct[rows[i].val] = true
		}
		if len(distinct) >= 3 && len(distinct) <= len(names)-1 {
			break
		}
	}
	sorted := append([]row{}, rows...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].val != sorted[j].val {
			if m.desc {
				return sorted[i].val > sorted[j].val
			}
			return sorted[i].val < sorted[j].val
		}
		return sorted[i].name < sorted[j].name
	})
	fn := itembank.Pick(r, []string{"RANK", "DENSE_RANK", "ROW_NUMBER"})
	ranks := make([]int, len(sorted))
	distinctBefore, firstSame := 0, 0
	for i, rw := range sorted {
		if i > 0 && rw.val != sorted[i-1].val {
			distinctBefore++
			firstSame = i
		}
		switch fn {
		case "RANK":
			ranks[i] = firstSame + 1
		case "DENSE_RANK":
			ranks[i] = distinctBefore + 1
		default:
			ranks[i] = i + 1
		}
	}
	dir := "ASC"
	if m.desc {
		dir = "DESC"
	}
	over := fmt.Sprintf("%s() OVER (ORDER BY %s %s)", fn, m.col, dir)
	if fn == "ROW_NUMBER" {
		over = fmt.Sprintf("ROW_NUMBER() OVER (ORDER BY %s %s, name)", m.col, dir)
	}
	filter := r.IntN(2) == 0
	k := itembank.IntBetween(r, 2, 3)
	query := fmt.Sprintf("SELECT name, %s AS rnk\nFROM %s\nORDER BY rnk, name;", over, table)
	if filter {
		query = fmt.Sprintf("SELECT name, rnk\nFROM (\n  SELECT name, %s AS rnk\n  FROM %s\n) s\nWHERE rnk <= %d\nORDER BY rnk, name;", over, table, k)
	}
	result := [][]string{}
	var ranked []string
	for i, rw := range sorted {
		ranked = append(ranked, fmt.Sprintf("%s (%d) → %d", rw.name, rw.val, ranks[i]))
		if !filter || ranks[i] <= k {
			result = append(result, []string{rw.name, strconv.Itoa(ranks[i])})
		}
	}
	var md [][]string
	for _, rw := range itembank.Shuffle(r, rows) {
		md = append(md, []string{rw.name, strconv.Itoa(rw.val)})
	}
	semantics := map[string]string{
		"RANK":       "RANK даёт равным значениям одинаковый номер и пропускает следующие номера (после двух первых мест идёт третье)",
		"DENSE_RANK": "DENSE_RANK даёт равным значениям одинаковый номер без пропусков (после двух первых мест идёт второе)",
		"ROW_NUMBER": "ROW_NUMBER нумерует строки подряд, а ничьи разрывает по второму ключу сортировки — name",
	}[fn]
	filterNote := ""
	if filter {
		filterNote = fmt.Sprintf(" Внешний запрос оставляет строки с rnk ≤ %d — оконную функцию нельзя фильтровать в WHERE того же уровня, поэтому нужен подзапрос.", k)
	}
	stems := []string{
		fmt.Sprintf("Таблица `%s`:\n\n%s\nКакие строки вернёт запрос (PostgreSQL)? Укажите все строки результата в порядке выдачи (name, rnk).", table, mdTable([]string{"name", m.col}, md)),
		fmt.Sprintf("Содержимое `%s`:\n\n%s\nПеречислите строки результата запроса в порядке выдачи (столбцы name, rnk).", table, mdTable([]string{"name", m.col}, md)),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer: itembank.Answer{Rows: result},
		Explanation: fmt.Sprintf("Строки сортируются по %s %s, при равенстве — по name: %s. %s.%s "+
			"Типичная ошибка — путать RANK и DENSE_RANK (пропуск номеров после ничьей) или нумеровать равные значения подряд, как ROW_NUMBER.",
			m.col, dir, strings.Join(ranked, ", "), semantics, filterNote),
	}, nil
}

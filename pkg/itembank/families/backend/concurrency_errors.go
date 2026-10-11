package backend

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(channelFilterSum{})
	itembank.Register(dataRaceVars{})
	itembank.Register(deferNamedReturn{})
	itembank.Register(logLevelFilter{})
}

// channelFilterSum — что выведет программа, читающая канал через range с фильтром.
type channelFilterSum struct{}

func (channelFilterSum) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.concurrency.channel-filter-sum.v1", Spec: itembank.SpecBackend, Topic: "concurrency",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.1, B: -0.5, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"go", "channels"},
	}
}

func (f channelFilterSum) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC401)
	capacity := itembank.IntBetween(r, 0, 4)
	n := itembank.IntBetween(r, 4, 9)
	step := itembank.IntBetween(r, 1, 4)
	acc := itembank.Pick(r, []string{"total", "sum", "acc", "result"})
	ch := itembank.Pick(r, []string{"ch", "out", "values", "stream"})
	m := itembank.IntBetween(r, 2, 4)
	threshold := itembank.IntBetween(r, 2, n-1) * step
	kind := r.IntN(3)
	var cond, condText string
	var keep func(v int) bool
	switch kind {
	case 0:
		cond = fmt.Sprintf("v%%%d == 0", m)
		condText = fmt.Sprintf("кратные %d", m)
		keep = func(v int) bool { return v%m == 0 }
	case 1:
		cond = fmt.Sprintf("v%%%d != 0", m)
		condText = fmt.Sprintf("не кратные %d", m)
		keep = func(v int) bool { return v%m != 0 }
	default:
		cond = fmt.Sprintf("v > %d", threshold)
		condText = fmt.Sprintf("больше %d", threshold)
		keep = func(v int) bool { return v > threshold }
	}
	total := 0
	var kept []int
	for i := 1; i <= n; i++ {
		v := i * step
		if keep(v) {
			total += v
			kept = append(kept, v)
		}
	}
	code := fmt.Sprintf(`package main

import "fmt"

func main() {
	%s := make(chan int, %d)
	go func() {
		for i := 1; i <= %d; i++ {
			%s <- i * %d
		}
		close(%s)
	}()
	%s := 0
	for v := range %s {
		if %s {
			%s += v
		}
	}
	fmt.Println(%s)
}`, ch, capacity, n, ch, step, ch, acc, ch, cond, acc, acc)
	stems := []string{
		"Что выведет программа?",
		"Какой результат напечатает программа после завершения?",
		"Что окажется в стандартном выводе программы?",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "go", Source: code},
		Answer: itembank.Answer{Text: strconv.Itoa(total)},
		Explanation: fmt.Sprintf("Горутина отправляет значения %d, %d, … (всего %d, шаг %d) и закрывает канал; range читает до закрытия, "+
			"а ёмкость буфера (%d) влияет только на то, где блокируется отправитель, но не на результат. Остаются значения %s: %s, сумма %d.",
			step, 2*step, n, step, capacity, condText, fmtInts(kept), total),
	}, nil
}

// dataRaceVars — у каких переменных есть гонка данных.
type dataRaceVars struct{}

func (dataRaceVars) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.concurrency.data-race-vars.v1", Spec: itembank.SpecBackend, Topic: "concurrency",
		Grade: itembank.GradeSenior, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.4, B: 1.0, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"go", "race"},
	}
}

type raceKind struct {
	racy    bool
	decl    func(v string, n int) string
	stmt    func(v string) string
	explain func(v string) string
	atomic  bool
	usesMu  bool
	usesFmt bool
}

var raceKinds = []raceKind{
	{racy: true,
		decl: func(v string, _ int) string { return v + " := 0" },
		stmt: func(v string) string { return v + " += i" },
		explain: func(v string) string {
			return "`" + v + "` изменяется из нескольких горутин без синхронизации — гонка"
		}},
	{racy: true,
		decl: func(v string, _ int) string { return "var " + v + " []int" },
		stmt: func(v string) string { return v + " = append(" + v + ", i)" },
		explain: func(v string) string {
			return "append к `" + v + "` перезаписывает заголовок среза из разных горутин — гонка"
		}},
	{racy: true,
		decl: func(v string, _ int) string { return v + " := map[int]bool{}" },
		stmt: func(v string) string { return v + "[i] = true" },
		explain: func(v string) string {
			return "конкурентная запись в map `" + v + "` без блокировки — гонка (и fatal error во время выполнения)"
		}},
	{racy: false, usesMu: true,
		decl: func(v string, _ int) string { return v + " := 0" },
		stmt: func(v string) string { return "mu.Lock()\n\t\t\t" + v + " += i\n\t\t\tmu.Unlock()" },
		explain: func(v string) string {
			return "`" + v + "` меняется только под мьютексом — гонки нет"
		}},
	{racy: false, atomic: true,
		decl: func(v string, _ int) string { return "var " + v + " int64" },
		stmt: func(v string) string { return "atomic.AddInt64(&" + v + ", 1)" },
		explain: func(v string) string {
			return "`" + v + "` обновляется атомарно — гонки нет"
		}},
	{racy: false, usesFmt: true,
		decl: func(v string, n int) string { return fmt.Sprintf("%s := %d", v, n/2) },
		stmt: func(v string) string { return "if i == " + v + " {\n\t\t\t\tfmt.Println(\"hit\")\n\t\t\t}" },
		explain: func(v string) string {
			return "`" + v + "` записана до запуска горутин и только читается — гонки нет"
		}},
	{racy: false,
		decl: func(v string, _ int) string { return v + " := make([]int, n)" },
		stmt: func(v string) string { return v + "[i] = i * 2" },
		explain: func(v string) string {
			return "каждая горутина пишет в свой элемент `" + v + "[i]`, элементы не пересекаются — гонки нет"
		}},
}

func (f dataRaceVars) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC402)
	n := itembank.IntBetween(r, 4, 16)
	racyCount := itembank.IntBetween(r, 2, 3)
	var racy, safe []raceKind
	for _, k := range raceKinds {
		if k.racy {
			racy = append(racy, k)
		} else {
			safe = append(safe, k)
		}
	}
	chosen := append(itembank.PickN(r, racy, racyCount), itembank.PickN(r, safe, 5-racyCount)...)
	chosen = itembank.Shuffle(r, chosen)
	// Имена не должны совпадать с n, i, mu, wg из каркаса программы.
	var names []string
	for _, v := range itembank.Idents(r, "var", 7) {
		if v != "n" && v != "i" && len(names) < 5 {
			names = append(names, v)
		}
	}
	imports := []string{"\"fmt\"", "\"sync\""}
	needAtomic, needMu := false, false
	for _, k := range chosen {
		needAtomic = needAtomic || k.atomic
		needMu = needMu || k.usesMu
	}
	if needAtomic {
		imports = append(imports, "\"sync/atomic\"")
	}
	var decls, stmts, prints, explains, correct []string
	for i, k := range chosen {
		v := names[i]
		decls = append(decls, "\t"+k.decl(v, n))
		stmts = append(stmts, "\t\t\t"+k.stmt(v))
		prints = append(prints, v)
		explains = append(explains, k.explain(v))
		if k.racy {
			correct = append(correct, "`"+v+"`")
		}
	}
	muDecl := ""
	if needMu {
		muDecl = "\tvar mu sync.Mutex\n"
	}
	code := fmt.Sprintf(`package main

import (
	%s
)

func main() {
	n := %d
	var wg sync.WaitGroup
%s%s
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
%s
		}(i)
	}
	wg.Wait()
	fmt.Println(%s)
}`, strings.Join(imports, "\n\t"), n, muDecl, strings.Join(decls, "\n"), strings.Join(stmts, "\n"), strings.Join(prints, ", "))
	var all []string
	for _, v := range names {
		all = append(all, "`"+v+"`")
	}
	var distractors []string
	for _, v := range all {
		isCorrect := false
		for _, c := range correct {
			if c == v {
				isCorrect = true
			}
		}
		if !isCorrect {
			distractors = append(distractors, v)
		}
	}
	options, keys := itembank.ShuffledMulti(r, correct, distractors...)
	stems := []string{
		fmt.Sprintf("Программа запускает %d горутин. Отметьте все переменные, доступ к которым содержит гонку данных (data race) по модели памяти Go.", n),
		"Какие из переменных программы изменяются с гонкой данных? Отметьте все подходящие.",
		fmt.Sprintf("Запуск с `-race` на программе с %d горутинами. Для каких переменных детектор сообщит о гонке? Отметьте все.", n),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "go", Source: code},
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: strings.Join(explains, "; ") + ". Чтение после wg.Wait() синхронизировано и гонки не создаёт. " +
			"Типичная ошибка — считать append и запись в map «атомарными» или видеть гонку в записи по разным индексам среза.",
	}, nil
}

// deferNamedReturn — что вернёт функция с именованным результатом и defer.
type deferNamedReturn struct{}

func (deferNamedReturn) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.errors_logging.defer-named-return.v1", Spec: itembank.SpecBackend, Topic: "errors_logging",
		Grade: itembank.GradeMiddle, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.3, B: 0.2, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"go", "defer", "panic"},
	}
}

func (f deferNamedReturn) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC403)
	fn := itembank.Ident(r, "func")
	res := itembank.Pick(r, []string{"res", "out", "n", "v"})
	x := itembank.IntBetween(r, 1, 9)
	c := itembank.IntBetween(r, 1, 5)
	withPanic := r.IntN(3) == 0
	limit := itembank.IntBetween(r, 1, 9)
	recoverDelta := itembank.IntBetween(r, 10, 90)
	type op struct {
		sym string
		k   int
	}
	nDefers := itembank.IntBetween(r, 2, 3)
	ops := make([]op, nDefers)
	for i := range ops {
		ops[i] = op{itembank.Pick(r, []string{"+", "-", "*"}), itembank.IntBetween(r, 2, 5)}
	}
	retMul := r.IntN(2) == 0
	retK := itembank.IntBetween(r, 2, 4)
	var lines []string
	lines = append(lines, "package main", "", "import \"fmt\"", "", fmt.Sprintf("func %s(x int) (%s int) {", fn, res))
	if withPanic {
		lines = append(lines, "\tdefer func() {", "\t\tif r := recover(); r != nil {", fmt.Sprintf("\t\t\t%s += %d", res, recoverDelta), "\t\t}", "\t}()")
	}
	for _, o := range ops {
		lines = append(lines, fmt.Sprintf("\tdefer func() { %s %s= %d }()", res, o.sym, o.k))
	}
	lines = append(lines, fmt.Sprintf("\t%s = x + %d", res, c))
	if withPanic {
		lines = append(lines, fmt.Sprintf("\tif x > %d {", limit), "\t\tpanic(\"limit\")", "\t}")
	}
	retSym := "+"
	if retMul {
		retSym = "*"
	}
	lines = append(lines, fmt.Sprintf("\treturn %s %s %d", res, retSym, retK), "}", "", "func main() {", fmt.Sprintf("\tfmt.Println(%s(%d))", fn, x), "}")

	// Вычисляем результат: присваивание, затем либо return, либо panic; defer в обратном порядке.
	value := x + c
	trace := []string{fmt.Sprintf("%s = %d", res, value)}
	panicked := withPanic && x > limit
	if panicked {
		trace = append(trace, "panic: return не выполняется")
	} else {
		if retMul {
			value *= retK
		} else {
			value += retK
		}
		trace = append(trace, fmt.Sprintf("return записывает %d", value))
	}
	for i := len(ops) - 1; i >= 0; i-- {
		switch ops[i].sym {
		case "+":
			value += ops[i].k
		case "-":
			value -= ops[i].k
		default:
			value *= ops[i].k
		}
		trace = append(trace, fmt.Sprintf("defer %s= %d → %d", ops[i].sym, ops[i].k, value))
	}
	if panicked {
		value += recoverDelta
		trace = append(trace, fmt.Sprintf("recover: += %d → %d", recoverDelta, value))
	} else if withPanic {
		trace = append(trace, "recover() возвращает nil, последний defer ничего не меняет")
	}
	stems := []string{
		"Что выведет программа?",
		fmt.Sprintf("Что напечатает вызов `%s(%d)`?", fn, x),
		"Какое число окажется в выводе программы?",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "go", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Text: strconv.Itoa(value)},
		Explanation: "Именованный результат меняют и return, и отложенные функции, которые выполняются в обратном порядке регистрации (LIFO) уже после return. " +
			"Порядок: " + strings.Join(trace, "; ") + ". Типичная ошибка — выполнять defer в порядке объявления или забыть, что return сначала присваивает значение.",
	}, nil
}

// logLevelFilter — сколько записей попадёт в лог при заданной конфигурации уровней.
type logLevelFilter struct{}

func (logLevelFilter) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.errors_logging.log-level-filter.v1", Spec: itembank.SpecBackend, Topic: "errors_logging",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.0, B: -0.9, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"logging", "config"},
	}
}

var logLevels = []string{"debug", "info", "warn", "error"}

func logLevelRank(l string) int {
	for i, x := range logLevels {
		if x == l {
			return i
		}
	}
	return -1
}

func (f logLevelFilter) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC404)
	modules := itembank.PickN(r, []string{"db", "http", "auth", "cache", "queue", "mail", "search"}, 3)
	global := itembank.Pick(r, []string{"info", "warn", "error"})
	overrides := map[string]string{}
	overridden := itembank.PickN(r, modules, itembank.IntBetween(r, 1, 2))
	for _, m := range overridden {
		for {
			l := itembank.Pick(r, logLevels)
			if l != global {
				overrides[m] = l
				break
			}
		}
	}
	var cfg strings.Builder
	fmt.Fprintf(&cfg, "logging:\n  level: %s\n  overrides:\n", global)
	for _, m := range modules {
		if l, ok := overrides[m]; ok {
			fmt.Fprintf(&cfg, "    %s: %s\n", m, l)
		}
	}
	messages := []string{"request started", "query planned", "token near expiry", "upstream failed", "cache miss", "retrying job",
		"connection reset", "slow query", "user not found", "payload too large", "config reloaded", "worker idle"}
	msgs := itembank.PickN(r, messages, 8)
	calls := itembank.IntBetween(r, 6, 8)
	var code []string
	var written []string
	for i := 0; i < calls; i++ {
		m := itembank.Pick(r, modules)
		l := itembank.Pick(r, logLevels)
		code = append(code, fmt.Sprintf("log.%s(\"%s\", \"%s\")", strings.ToUpper(l[:1])+l[1:], m, msgs[i]))
		eff := global
		if o, ok := overrides[m]; ok {
			eff = o
		}
		if logLevelRank(l) >= logLevelRank(eff) {
			written = append(written, fmt.Sprintf("%s/%s", m, l))
		}
	}
	var ovParts []string
	for _, m := range modules {
		if l, ok := overrides[m]; ok {
			ovParts = append(ovParts, m+" → "+l)
		}
	}
	stems := []string{
		"Конфигурация логгера (`overrides` задаёт порог для отдельных модулей, остальные используют `level`; запись проходит, если её уровень не ниже порога):\n\n```yaml\n" + cfg.String() + "```\n\nСколько записей из приведённого кода попадут в лог? Первый аргумент — модуль.",
		"Логгер читает конфигурацию ниже: порог модуля берётся из `overrides`, иначе из `level`; уровни по возрастанию — debug, info, warn, error.\n\n```yaml\n" + cfg.String() + "```\n\nСколько вызовов ниже реально запишут строку в лог (первый аргумент — имя модуля)?",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "go", Source: strings.Join(code, "\n")},
		Answer: itembank.Answer{Number: float64(len(written))},
		Explanation: fmt.Sprintf("Порог по умолчанию %s, переопределения: %s. Проходят записи с уровнем не ниже порога своего модуля: %s — всего %d. "+
			"Типичная ошибка — применять глобальный порог к модулю с переопределением или считать, что warn ниже info.",
			global, strings.Join(ovParts, ", "), strings.Join(written, ", "), len(written)),
	}, nil
}

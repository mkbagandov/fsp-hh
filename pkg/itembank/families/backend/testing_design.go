package backend

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(branchCoverage{})
	itembank.Register(cleanupOrder{})
	itembank.Register(littleLawCapacity{})
	itembank.Register(reshardingMoves{})
	itembank.Register(selectDefaultDrops{})
}

// branchCoverage — сколько операторов return функции покроет табличный тест.
type branchCoverage struct{}

func (branchCoverage) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.testing.branch-coverage.v1", Spec: itembank.SpecBackend, Topic: "testing",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: -0.1, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"go", "coverage", "table-tests"},
	}
}

var coverageLabelSets = [][]string{
	{"gold", "silver", "bronze", "basic", "none"},
	{"A", "B", "C", "D", "E"},
	{"high", "elevated", "medium", "low", "minimal"},
	{"vip", "premium", "standard", "economy", "guest"},
}

func (f branchCoverage) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE501)
	fn := itembank.Pick(r, []string{"tier", "grade", "bucket", "band", "level"})
	arg := itembank.Pick(r, []string{"score", "points", "value", "amount"})
	k := itembank.IntBetween(r, 3, 4)
	guard := k == 3 && r.IntN(2) == 0
	pool := make([]int, 0, 18)
	for v := 10; v <= 95; v += 5 {
		pool = append(pool, v)
	}
	thresholds := itembank.PickN(r, pool, k)
	sort.Sort(sort.Reverse(sort.IntSlice(thresholds)))
	labels := itembank.Pick(r, coverageLabelSets)[:k+1]
	total := k + 1
	if guard {
		total++
	}
	// Индекс return: 0..k-1 — пороги сверху вниз, k — ветка по умолчанию, k+1 — проверка диапазона.
	branchOf := func(x int) int {
		if guard && (x < 0 || x > 100) {
			return k + 1
		}
		for i, th := range thresholds {
			if x >= th {
				return i
			}
		}
		return k
	}
	labelOf := func(b int) string {
		if b == k+1 {
			return "invalid"
		}
		return labels[b]
	}
	coveredBy := func(xs []int) map[int]bool {
		m := map[int]bool{}
		for _, x := range xs {
			m[branchOf(x)] = true
		}
		return m
	}
	var inputs []int
	for attempt := 0; attempt < 100; attempt++ {
		inputs = inputs[:0]
		seen := map[int]bool{}
		n := itembank.IntBetween(r, 3, 5)
		for len(inputs) < n {
			var x int
			switch r.IntN(5) {
			case 0:
				x = itembank.Pick(r, thresholds)
			case 1:
				x = itembank.Pick(r, thresholds) - 1
			case 2:
				x = itembank.IntBetween(r, -15, 125)
			default:
				x = itembank.IntBetween(r, 0, 100)
			}
			if seen[x] {
				continue
			}
			seen[x] = true
			inputs = append(inputs, x)
		}
		if len(coveredBy(inputs)) < total {
			break
		}
	}
	for len(coveredBy(inputs)) >= total {
		inputs = inputs[:len(inputs)-1]
	}
	covered := coveredBy(inputs)

	fnLines := []string{fmt.Sprintf("func %s(%s int) string {", fn, arg)}
	if guard {
		fnLines = append(fnLines, fmt.Sprintf("\tif %s < 0 || %s > 100 {", arg, arg), "\t\treturn \"invalid\"", "\t}")
	}
	for i, th := range thresholds {
		fnLines = append(fnLines, fmt.Sprintf("\tif %s >= %d {", arg, th), fmt.Sprintf("\t\treturn %q", labels[i]), "\t}")
	}
	fnLines = append(fnLines, fmt.Sprintf("\treturn %q", labels[k]), "}")
	var cases, trace []string
	for _, x := range inputs {
		cases = append(cases, fmt.Sprintf("%d: %q", x, labelOf(branchOf(x))))
		trace = append(trace, fmt.Sprintf("%d → %s", x, labelOf(branchOf(x))))
	}
	testName := "Test" + strings.ToUpper(fn[:1]) + fn[1:]
	code := strings.Join(fnLines, "\n") + fmt.Sprintf(`

func %s(t *testing.T) {
	cases := map[int]string{%s}
	for in, want := range cases {
		if got := %s(in); got != want {
			t.Errorf("%s(%%d) = %%q, want %%q", in, got, want)
		}
	}
}`, testName, strings.Join(cases, ", "), fn, fn)

	var missed []string
	for b := 0; b < total; b++ {
		if !covered[b] {
			missed = append(missed, "`return \""+labelOf(b)+"\"`")
		}
	}
	uncovered := total - len(covered)
	percent := math.Round(100 * float64(len(covered)) / float64(total))
	var question string
	var answer, tol float64
	switch r.IntN(3) {
	case 0:
		question = fmt.Sprintf("Сколько операторов `return` функции `%s` не выполнится ни разу при прогоне теста?", fn)
		answer = float64(uncovered)
	case 1:
		question = fmt.Sprintf("Какой процент операторов `return` функции `%s` будет покрыт тестом? Ответ — целое число процентов (округлите до ближайшего целого).", fn)
		answer, tol = percent, 0.5
	default:
		question = "Сколько кейсов минимум нужно добавить в таблицу, чтобы каждый оператор `return` функции выполнился хотя бы один раз?"
		answer = float64(uncovered)
	}
	intros := []string{
		fmt.Sprintf("Функция `%s` и табличный тест к ней приведены ниже; тест проходит (ожидания верны).", fn),
		fmt.Sprintf("Ниже реализация `%s` и её тест, все ожидания в таблице корректны.", fn),
	}
	guardNote := "; проверки диапазона в функции нет, поэтому значения вне 0–100 обрабатываются обычными порогами"
	if guard {
		guardNote = ", а значения вне 0–100 перехватывает первая проверка"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, intros) + " " + question,
		Code:   &itembank.CodeBlock{Lang: "go", Source: code},
		Answer: itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: fmt.Sprintf("Каждый кейс выполняет ровно один `return`: %s. Всего операторов return — %d, покрыто %d (%.0f %%), не выполняются: %s. "+
			"Условия проверяются сверху вниз с `>=`, поэтому значение, равное порогу, уходит в верхнюю ветку%s. "+
			"Типичная ошибка — считать покрытие по числу кейсов или относить граничное значение к нижней ветке.",
			strings.Join(trace, ", "), total, len(covered), percent, strings.Join(missed, ", "), guardNote),
	}, nil
}

// cleanupOrder — порядок вывода теста с t.Cleanup, defer и подтестами.
type cleanupOrder struct{}

func (cleanupOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.testing.cleanup-order.v1", Spec: itembank.SpecBackend, Topic: "testing",
		Grade: itembank.GradeSenior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.5, B: 0.8, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"go", "testing", "cleanup"},
	}
}

func (f cleanupOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE502)
	testName := itembank.Pick(r, []string{"TestPipeline", "TestRepository", "TestMigrations", "TestOrders", "TestImport", "TestCache"})
	subNames := itembank.PickN(r, []string{"insert", "select", "update", "delete", "migrate", "seed", "index", "export"}, itembank.IntBetween(r, 2, 3))
	parentCleanups := itembank.PickN(r, []string{"close db", "drop schema", "stop server", "remove tmp"}, itembank.IntBetween(r, 1, 2))
	parentDefer := r.IntN(3) > 0
	printStart := r.IntN(2) == 0
	printEnd := r.IntN(3) > 0
	type sub struct {
		name                           string
		cleanup, deferred, cleanupLate bool
	}
	subs := make([]sub, len(subNames))
	anyCleanup := false
	for i, name := range subNames {
		subs[i] = sub{name: name, cleanup: r.IntN(3) > 0, deferred: r.IntN(2) == 0, cleanupLate: r.IntN(2) == 0}
		anyCleanup = anyCleanup || subs[i].cleanup
	}
	if !anyCleanup {
		subs[0].cleanup = true
	}
	lines := []string{fmt.Sprintf("func %s(t *testing.T) {", testName)}
	for _, c := range parentCleanups {
		lines = append(lines, fmt.Sprintf("\tt.Cleanup(func() { fmt.Println(%q) })", c))
	}
	if parentDefer {
		lines = append(lines, "\tdefer fmt.Println(\"defer parent\")")
	}
	if printStart {
		lines = append(lines, "\tfmt.Println(\"start\")")
	}
	for _, s := range subs {
		lines = append(lines, fmt.Sprintf("\tt.Run(%q, func(t *testing.T) {", s.name))
		cleanupLine := fmt.Sprintf("\t\tt.Cleanup(func() { fmt.Println(\"cleanup %s\") })", s.name)
		if s.cleanup && !s.cleanupLate {
			lines = append(lines, cleanupLine)
		}
		if s.deferred {
			lines = append(lines, fmt.Sprintf("\t\tdefer fmt.Println(\"defer %s\")", s.name))
		}
		lines = append(lines, fmt.Sprintf("\t\tfmt.Println(\"run %s\")", s.name))
		if s.cleanup && s.cleanupLate {
			lines = append(lines, cleanupLine)
		}
		lines = append(lines, "\t})")
	}
	if printEnd {
		lines = append(lines, "\tfmt.Println(\"end\")")
	}
	lines = append(lines, "}")

	// Вывод: тело → defer → cleanup для каждого подтеста, затем остаток родителя, его defer и cleanup (LIFO).
	var out []string
	if printStart {
		out = append(out, "start")
	}
	for _, s := range subs {
		out = append(out, "run "+s.name)
		if s.deferred {
			out = append(out, "defer "+s.name)
		}
		if s.cleanup {
			out = append(out, "cleanup "+s.name)
		}
	}
	if printEnd {
		out = append(out, "end")
	}
	if parentDefer {
		out = append(out, "defer parent")
	}
	for i := len(parentCleanups) - 1; i >= 0; i-- {
		out = append(out, parentCleanups[i])
	}
	lifoNote := ""
	if len(parentCleanups) == 2 {
		lifoNote = " Два cleanup родителя выполняются в обратном порядке регистрации: сначала «" + parentCleanups[1] + "», потом «" + parentCleanups[0] + "»."
	}
	stems := []string{
		fmt.Sprintf("Тест ниже запускают командой `go test -v -run %s`. Какие строки и в каком порядке напечатают вызовы `fmt.Println`? Служебные строки `=== RUN`, `--- PASS` и `PASS` не учитывайте; укажите по одной строке вывода на строку ответа.", testName),
		fmt.Sprintf("Что выведут вызовы `fmt.Println` при выполнении теста `%s` (без служебного вывода самого `go test`)? Перечислите строки в порядке появления, по одной на строку.", testName),
		"В каком порядке появятся сообщения теста в stdout? Приведите вывод построчно, не включая строки, которые печатает сам `go test`.",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "go", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Text: strings.Join(out, "\n")},
		Explanation: "`defer` срабатывает при возврате из функции, а функции `t.Cleanup` — после завершения теста и всех его подтестов, в порядке «последний добавлен — первый вызван». " +
			"Подтест в `t.Run` выполняется целиком (тело → его defer → его cleanup), и только потом управление возвращается родителю; defer родителя выполняется раньше его cleanup. " +
			"Порядок: " + strings.Join(out, " → ") + "." + lifoNote +
			" Типичная ошибка — откладывать cleanup подтестов до конца всего теста или ставить cleanup родителя раньше его defer.",
	}, nil
}

// littleLawCapacity — закон Литтла: одновременные запросы, число экземпляров или предельный RPS.
type littleLawCapacity struct{}

func (littleLawCapacity) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.system_design.little-law-capacity.v1", Spec: itembank.SpecBackend, Topic: "system_design",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.2, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"capacity", "littles-law"},
	}
}

func (f littleLawCapacity) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE503)
	svc := itembank.Ident(r, "service")
	rps := itembank.Pick(r, []int{200, 300, 400, 500, 600, 800, 1000, 1200, 1500, 2000, 2400, 3000})
	latency := itembank.Pick(r, []int{20, 25, 40, 50, 80, 100, 120, 150, 200, 250})
	workers := itembank.Pick(r, []int{8, 10, 16, 20, 25, 32, 40, 50, 64})
	util := itembank.Pick(r, []int{50, 60, 70, 75, 80})
	instances := itembank.IntBetween(r, 2, 12)
	latencySec := float64(latency) / 1000
	inflight := float64(rps*latency) / 1000
	var stems []string
	var answer, tol float64
	var explain string
	switch r.IntN(3) {
	case 0:
		answer, tol = math.Round(inflight*100)/100, 0.01
		stems = []string{
			fmt.Sprintf("Сервис `%s` получает в среднем %d запросов в секунду, средняя длительность обработки запроса — %d мс. Сколько запросов в среднем находятся в обработке одновременно? Ответ укажите с точностью до сотых.", svc, rps, latency),
			fmt.Sprintf("Нагрузка на `%s` — %d RPS при средней задержке %d мс. Какова средняя одновременная нагрузка (число запросов «в полёте»)? Ответ — с точностью до сотых.", svc, rps, latency),
		}
		explain = fmt.Sprintf("По закону Литтла L = λ·W: %d запросов/с × %.3f с = %.2f одновременных запросов. "+
			"Типичная ошибка — забыть перевести миллисекунды в секунды или разделить λ на W.", rps, latencySec, inflight)
	case 1:
		num, den := rps*latency, 10*workers*util
		need := (num + den - 1) / den
		if need < 2 {
			// Один экземпляр — вырожденный случай: поднимаем нагрузку, чтобы ответ требовал расчёта.
			rps = itembank.Pick(r, []int{1500, 2000, 2400, 3000})
			workers = itembank.Pick(r, []int{8, 10, 16})
			inflight = float64(rps*latency) / 1000
			num, den = rps*latency, 10*workers*util
			need = (num + den - 1) / den
		}
		answer = float64(need)
		perInstance := float64(workers*util) / 100
		stems = []string{
			fmt.Sprintf("Каждый экземпляр `%s` держит пул из %d воркеров (столько запросов он обрабатывает одновременно). Нужно обслуживать %d RPS при средней задержке %d мс так, чтобы средняя загрузка пулов не превышала %d %%. Сколько экземпляров нужно минимум?", svc, workers, rps, latency, util),
			fmt.Sprintf("Сервис `%s` планируют под %d запросов в секунду с задержкой %d мс; экземпляр обрабатывает до %d запросов параллельно, а целевая средняя утилизация — %d %%. Какое минимальное число экземпляров закрывает нагрузку?", svc, rps, latency, workers, util),
		}
		explain = fmt.Sprintf("Закон Литтла: одновременно в обработке L = λ·W = %d × %.3f = %.2f запросов. Один экземпляр при загрузке %d %% даёт %d × %.2f = %.2f полезных слотов, "+
			"значит минимальное число экземпляров — ⌈%.2f / %.2f⌉ = %d. Типичная ошибка — округлять вниз или делить RPS на число воркеров, забыв про задержку.",
			rps, latencySec, inflight, util, workers, float64(util)/100, perInstance, inflight, perInstance, need)
	default:
		maxRPS := float64(instances*workers) / latencySec
		answer, tol = math.Round(maxRPS*100)/100, 0.01
		stems = []string{
			fmt.Sprintf("Развёрнуто %d экземпляров `%s`, в каждом пул из %d воркеров; средняя задержка одного запроса — %d мс. Какой максимальный средний RPS выдержит система при полной загрузке всех воркеров? Ответ с точностью до сотых.", instances, svc, workers, latency),
			fmt.Sprintf("У `%s` %d реплик по %d одновременных запросов каждая, обработка занимает в среднем %d мс. Какую пропускную способность (запросов в секунду) даёт такая конфигурация при 100 %% загрузке? Ответ — с точностью до сотых.", svc, instances, workers, latency),
		}
		explain = fmt.Sprintf("Из закона Литтла λ = L / W: всего слотов L = %d × %d = %d, W = %.3f с, λ = %d / %.3f = %.2f запросов в секунду. "+
			"Типичная ошибка — умножать слоты на задержку вместо деления или оставлять задержку в миллисекундах.",
			instances, workers, instances*workers, latencySec, instances*workers, latencySec, maxRPS)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        itembank.Pick(r, stems),
		Answer:      itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: explain,
	}, nil
}

// reshardingMoves — сколько ключей переедут при изменении числа узлов (кольцо или hash mod N).
type reshardingMoves struct{}

func (reshardingMoves) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.system_design.resharding-moves.v1", Spec: itembank.SpecBackend, Topic: "system_design",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.7, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"sharding", "consistent-hashing"},
	}
}

type ringNode struct {
	name string
	pos  int
}

// ringOwner — первый узел по часовой стрелке (позиция ≥ pos), с переходом через ноль к первому.
func ringOwner(pos int, nodes []ringNode) string {
	for _, n := range nodes {
		if n.pos >= pos {
			return n.name
		}
	}
	return nodes[0].name
}

func sortRing(nodes []ringNode) []ringNode {
	out := append([]ringNode{}, nodes...)
	sort.Slice(out, func(i, j int) bool { return out[i].pos < out[j].pos })
	return out
}

func (f reshardingMoves) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE504)
	prefix := itembank.Pick(r, []string{"user", "order", "cart", "session", "doc"})
	nKeys := itembank.IntBetween(r, 6, 8)
	keyNames := make([]string, nKeys)
	for i := range keyNames {
		keyNames[i] = fmt.Sprintf("%s:%d", prefix, i+1)
	}
	if r.IntN(3) == 2 {
		return f.modInstance(r, seed, keyNames)
	}
	ringSize := itembank.Pick(r, []int{100, 360})
	names := itembank.Pick(r, [][]string{{"A", "B", "C", "D", "E"}, {"n1", "n2", "n3", "n4", "n5"}})
	nNodes := itembank.IntBetween(r, 3, 4)
	pool := make([]int, 0, ringSize)
	for p := 1; p < ringSize; p++ {
		pool = append(pool, p)
	}
	addNode := r.IntN(2) == 0
	var nodes, after []ringNode
	var keyPos []int
	var action, actionShort string
	moved := 0
	var trace []string
	// Перебираем расстановки, пока хотя бы один ключ не сменит владельца: ответ «0» слишком легко угадать.
	for attempt := 0; attempt < 50 && moved == 0; attempt++ {
		positions := itembank.PickN(r, pool, nNodes+1+nKeys)
		nodePos := append([]int{}, positions[:nNodes]...)
		sort.Ints(nodePos)
		nodes = make([]ringNode, nNodes)
		for i, p := range nodePos {
			nodes[i] = ringNode{names[i], p}
		}
		keyPos = positions[nNodes+1:]
		if addNode {
			newNode := ringNode{names[nNodes], positions[nNodes]}
			after = sortRing(append(append([]ringNode{}, nodes...), newNode))
			action = fmt.Sprintf("В кольцо добавляют узел %s в позицию %d.", newNode.name, newNode.pos)
			actionShort = fmt.Sprintf("добавляется узел %s = %d", newNode.name, newNode.pos)
		} else {
			idx := r.IntN(nNodes)
			after = append(append([]ringNode{}, nodes[:idx]...), nodes[idx+1:]...)
			action = fmt.Sprintf("Узел %s (позиция %d) выводят из кольца.", nodes[idx].name, nodes[idx].pos)
			actionShort = fmt.Sprintf("удаляется узел %s = %d", nodes[idx].name, nodes[idx].pos)
		}
		moved, trace = 0, trace[:0]
		for i, p := range keyPos {
			before, now := ringOwner(p, nodes), ringOwner(p, after)
			mark := ""
			if before != now {
				moved++
				mark = " ★"
			}
			trace = append(trace, fmt.Sprintf("%s (%d): %s → %s%s", keyNames[i], p, before, now, mark))
		}
	}
	var nodeDesc, keyDesc []string
	for _, n := range nodes {
		nodeDesc = append(nodeDesc, fmt.Sprintf("%s = %d", n.name, n.pos))
	}
	for i, p := range keyPos {
		keyDesc = append(keyDesc, fmt.Sprintf("%s = %d", keyNames[i], p))
	}
	stems := []string{
		fmt.Sprintf("Кольцо consistent hashing размечено позициями 0–%d. Ключ обслуживает первый узел по часовой стрелке от его позиции — узел с наименьшей позицией, не меньшей позиции ключа; если такого нет, ключ достаётся узлу с наименьшей позицией. "+
			"Узлы: %s. Хеши ключей: %s. %s Сколько ключей сменят узел-владелец?", ringSize-1, strings.Join(nodeDesc, ", "), strings.Join(keyDesc, ", "), action),
		fmt.Sprintf("На хеш-кольце из %d позиций (0–%d) узлы стоят в точках %s, ключи попадают в точки %s; владелец ключа — ближайший по часовой стрелке узел (позиция ≥ позиции ключа, с переходом через 0 к первому узлу). "+
			"%s Для скольких ключей владелец изменится?", ringSize, ringSize-1, strings.Join(nodeDesc, ", "), strings.Join(keyDesc, ", "), action),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Answer: itembank.Answer{Number: float64(moved)},
		Explanation: fmt.Sprintf("Владелец ключа — первый узел по часовой стрелке; когда %s, переезжают только ключи из дуги между соседним узлом и изменившимся, остальные остаются на месте: %s. "+
			"Переедут %d из %d ключей (★). Типичная ошибка — брать ближайший узел против часовой стрелки или считать, что перераспределяются все ключи, как при `hash mod N`.",
			actionShort, strings.Join(trace, "; "), moved, nKeys),
	}, nil
}

// modInstance — вариант с шардированием по остатку: hash mod N → hash mod (N+1).
func (f reshardingMoves) modInstance(r *rand.Rand, seed uint64, keyNames []string) (itembank.Instance, error) {
	n := itembank.IntBetween(r, 3, 5)
	pool := make([]int, 0, 87)
	for h := 11; h <= 97; h++ {
		pool = append(pool, h)
	}
	hashes := itembank.PickN(r, pool, len(keyNames))
	moved := 0
	var rows [][]string
	var trace []string
	for i, h := range hashes {
		before, now := h%n, h%(n+1)
		mark := ""
		if before != now {
			moved++
			mark = " ★"
		}
		rows = append(rows, []string{keyNames[i], strconv.Itoa(h)})
		trace = append(trace, fmt.Sprintf("%s: %d mod %d = %d, %d mod %d = %d%s", keyNames[i], h, n, before, h, n+1, now, mark))
	}
	table := mdTable([]string{"key", "hash"}, rows)
	stems := []string{
		fmt.Sprintf("Ключи распределяются по %d шардам по правилу `shard = hash mod %d`. Число шардов увеличивают до %d (правило становится `hash mod %d`), после чего каждый ключ переносят туда, где он теперь должен лежать. Сколько ключей из таблицы сменят шард?\n\n%s", n, n, n+1, n+1, table),
		fmt.Sprintf("Шардирование по остатку: ключ с хешем h хранится на шарде `h mod N`. При добавлении шарда N меняется с %d на %d. Для скольких ключей из таблицы номер шарда изменится?\n\n%s", n, n+1, table),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Answer: itembank.Answer{Number: float64(moved)},
		Explanation: fmt.Sprintf("При шардировании по остатку смена N меняет остаток почти у всех ключей: %s. Переедут %d из %d ключей (★). "+
			"Типичная ошибка — ожидать, что переедет лишь около 1/(N+1) ключей, как при consistent hashing; остаются на месте только ключи с hash mod N = hash mod (N+1).",
			strings.Join(trace, "; "), moved, len(keyNames)),
	}, nil
}

// selectDefaultDrops — что выведет программа с неблокирующей отправкой в буферизованный канал.
type selectDefaultDrops struct{}

func (selectDefaultDrops) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.concurrency.select-default-drops.v1", Spec: itembank.SpecBackend, Topic: "concurrency",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.5, B: -0.5, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"go", "channels", "select"},
	}
}

func (f selectDefaultDrops) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xE505)
	ch := itembank.Pick(r, []string{"queue", "jobs", "events", "buf", "ch"})
	cnt := itembank.Pick(r, []string{"dropped", "lost", "skipped", "rejected"})
	capacity := itembank.IntBetween(r, 2, 4)
	n := itembank.IntBetween(r, 6, 10)
	printGot := r.IntN(2) == 0
	var cond string
	var drain func(i int) bool
	if r.IntN(2) == 0 {
		m := itembank.IntBetween(r, 2, 4)
		cond = fmt.Sprintf("i%%%d == 0", m)
		drain = func(i int) bool { return i%m == 0 }
	} else {
		idx := make([]int, 0, n-1)
		for i := 2; i <= n; i++ {
			idx = append(idx, i)
		}
		ab := itembank.PickN(r, idx, 2)
		sort.Ints(ab)
		a, b := ab[0], ab[1]
		cond = fmt.Sprintf("i == %d || i == %d", a, b)
		drain = func(i int) bool { return i == a || i == b }
	}
	recv := "<-" + ch
	if printGot {
		recv = fmt.Sprintf("fmt.Println(\"got\", <-%s)", ch)
	}
	code := fmt.Sprintf(`package main

import "fmt"

func main() {
	%s := make(chan int, %d)
	%s := 0
	for i := 1; i <= %d; i++ {
		select {
		case %s <- i:
		default:
			%s++
		}
		if %s {
			%s
		}
	}
	fmt.Println(len(%s), %s)
}`, ch, capacity, cnt, n, ch, cnt, cond, recv, ch, cnt)

	// Симуляция: после попытки отправки в буфере всегда есть хотя бы один элемент, поэтому приём не блокируется.
	var queue []int
	dropped := 0
	var out, trace []string
	for i := 1; i <= n; i++ {
		var step string
		if len(queue) < capacity {
			queue = append(queue, i)
			step = fmt.Sprintf("i=%d: буфер %s", i, fmtInts(queue))
		} else {
			dropped++
			step = fmt.Sprintf("i=%d: буфер полон, %s = %d", i, cnt, dropped)
		}
		if drain(i) {
			v := queue[0]
			queue = queue[1:]
			step += fmt.Sprintf(", получено %d", v)
			if printGot {
				out = append(out, fmt.Sprintf("got %d", v))
			}
		}
		trace = append(trace, step)
	}
	out = append(out, fmt.Sprintf("%d %d", len(queue), dropped))
	stems := []string{
		"Что выведет программа? Укажите все строки вывода по порядку.",
		"Какие строки напечатает программа после завершения `main` (по одной на строку ответа)?",
		"Что окажется в стандартном выводе программы? Перечислите строки в порядке появления.",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "go", Source: code},
		Answer: itembank.Answer{Text: strings.Join(out, "\n")},
		Explanation: fmt.Sprintf("`select` с ветвью `default` не блокируется: отправка проходит, пока в буфере (ёмкость %d) есть место, иначе выполняется default и счётчик растёт. "+
			"`<-%s` забирает самый старый элемент (FIFO), а `len` возвращает число элементов в буфере, не ёмкость. Трассировка: %s. Итог: в буфере %d, отброшено %d. "+
			"Типичная ошибка — считать, что отправка в полный канал внутри select дождётся освобождения, или путать len и cap.",
			capacity, ch, strings.Join(trace, "; "), len(queue), dropped),
	}, nil
}

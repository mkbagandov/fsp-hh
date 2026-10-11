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
	itembank.Register(savepointBalance{})
	itembank.Register(deadlockPair{})
	itembank.Register(isolationVisibleValue{})
	itembank.Register(nullAggregate{})
	itembank.Register(windowTopPerGroup{})
}

// mdTable строит Markdown-таблицу с заголовком и строками.
func mdTable(header []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString("| " + strings.Join(header, " | ") + " |\n|")
	for range header {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, row := range rows {
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	return b.String()
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// savepointBalance — значение поля после транзакции с SAVEPOINT / ROLLBACK TO / RELEASE.
type savepointBalance struct{}

func (savepointBalance) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.transactions.savepoint-balance.v1", Spec: itembank.SpecBackend, Topic: "transactions",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.8, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"sql", "postgres"},
	}
}

func (f savepointBalance) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x7A01)
	table := itembank.Pick(r, []string{"accounts", "wallets", "balances", "ledger", "deposits"})
	col := itembank.Pick(r, []string{"balance", "amount", "credits", "points"})
	id := itembank.IntBetween(r, 2, 99)
	start := itembank.IntBetween(r, 10, 50) * 10
	value := start
	stmts := []string{"BEGIN;"}
	trace := []string{fmt.Sprintf("старт %d", start)}
	update := func() {
		d := itembank.IntBetween(r, 1, 9) * 10
		op := "+"
		if r.IntN(2) == 0 {
			d, op = -d, "-"
		}
		stmts = append(stmts, fmt.Sprintf("UPDATE %s SET %s = %s %s %d WHERE id = %d;", table, col, col, op, absInt(d), id))
		value += d
		trace = append(trace, fmt.Sprintf("UPDATE → %d", value))
	}
	update()
	blocks := itembank.IntBetween(r, 1, 2)
	for i := 1; i <= blocks; i++ {
		sp := fmt.Sprintf("sp%d", i)
		stmts = append(stmts, fmt.Sprintf("SAVEPOINT %s;", sp))
		saved := value
		trace = append(trace, fmt.Sprintf("SAVEPOINT %s запоминает %d", sp, value))
		update()
		if r.IntN(3) > 0 {
			update()
		}
		if r.IntN(4) > 0 {
			stmts = append(stmts, fmt.Sprintf("ROLLBACK TO SAVEPOINT %s;", sp))
			value = saved
			trace = append(trace, fmt.Sprintf("ROLLBACK TO %s возвращает %d", sp, value))
		} else {
			stmts = append(stmts, fmt.Sprintf("RELEASE SAVEPOINT %s;", sp))
			trace = append(trace, fmt.Sprintf("RELEASE %s ничего не отменяет, остаётся %d", sp, value))
		}
	}
	if r.IntN(3) > 0 {
		update()
	}
	if r.IntN(5) > 0 {
		stmts = append(stmts, "COMMIT;")
		trace = append(trace, fmt.Sprintf("COMMIT фиксирует %d", value))
	} else {
		stmts = append(stmts, "ROLLBACK;")
		value = start
		trace = append(trace, fmt.Sprintf("ROLLBACK отменяет всю транзакцию → %d", start))
	}
	stems := []string{
		fmt.Sprintf("Строка `id = %d` таблицы `%s` имеет `%s = %d`. В одной сессии PostgreSQL выполняется последовательность команд ниже. Какое значение `%s` увидит другая сессия после её завершения?", id, table, col, start, col),
		fmt.Sprintf("В таблице `%s` у записи с `id = %d` поле `%s` равно %d. Клиент отправил команды ниже (PostgreSQL). Чему будет равно `%s` этой записи после последней команды?", table, id, col, start, col),
		fmt.Sprintf("Исходное значение `%s.%s` для `id = %d` — %d. После выполнения блока команд (PostgreSQL) чему равно `%s` в этой строке с точки зрения остальных сессий?", table, col, id, start, col),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: "sql", Source: strings.Join(stmts, "\n")},
		Answer: itembank.Answer{Number: float64(value)},
		Explanation: "Трассировка: " + strings.Join(trace, "; ") + ". ROLLBACK TO SAVEPOINT отменяет только изменения после точки сохранения, " +
			"RELEASE SAVEPOINT лишь удаляет точку и ничего не откатывает, а полный ROLLBACK отменяет всю транзакцию целиком.",
	}, nil
}

// deadlockPair — какая пара транзакций может взаимно заблокироваться.
type deadlockPair struct{}

func (deadlockPair) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.transactions.deadlock-pair.v1", Spec: itembank.SpecBackend, Topic: "transactions",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.3, C: 0.25}, TimeLimit: 150 * time.Second, Tags: []string{"sql", "locks"},
	}
}

// canDeadlock — две транзакции могут зациклиться, если есть пара строк, которые они
// захватывают в противоположном порядке.
func canDeadlock(a, b []int) bool {
	pos := func(xs []int, v int) int {
		for i, x := range xs {
			if x == v {
				return i
			}
		}
		return -1
	}
	for i, x := range a {
		for j, y := range a {
			if i >= j {
				continue
			}
			px, py := pos(b, x), pos(b, y)
			if px >= 0 && py >= 0 && py < px {
				return true
			}
		}
	}
	return false
}

func (f deadlockPair) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x7A02)
	table := itembank.Pick(r, []string{"accounts", "orders", "items", "inventory", "users", "tickets"})
	pool := make([]int, 0, 12)
	for i := 1; i <= 12; i++ {
		pool = append(pool, i)
	}
	ids := itembank.PickN(r, pool, 4)
	sort.Ints(ids)
	const n = 4
	locks := make([][]int, n)
	pair := itembank.PickN(r, []int{0, 1, 2, 3}, 2)
	xy := itembank.PickN(r, ids, 2)
	sort.Ints(xy)
	locks[pair[0]] = []int{xy[0], xy[1]}
	locks[pair[1]] = []int{xy[1], xy[0]}
	for t := 0; t < n; t++ {
		if locks[t] != nil {
			continue
		}
		for {
			pq := itembank.PickN(r, ids, 2)
			sort.Ints(pq)
			if pq[0] != xy[0] || pq[1] != xy[1] {
				locks[t] = pq
				break
			}
		}
	}
	useSelect := r.IntN(2) == 0
	col := itembank.Pick(r, []string{"balance", "qty", "status", "version"})
	var code strings.Builder
	for t := 0; t < n; t++ {
		fmt.Fprintf(&code, "-- T%d\nBEGIN;\n", t+1)
		for _, id := range locks[t] {
			if useSelect {
				fmt.Fprintf(&code, "SELECT * FROM %s WHERE id = %d FOR UPDATE;\n", table, id)
			} else {
				fmt.Fprintf(&code, "UPDATE %s SET %s = %s + 1 WHERE id = %d;\n", table, col, col, id)
			}
		}
		code.WriteString("COMMIT;\n")
	}
	var correct string
	var all []string
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			name := fmt.Sprintf("T%d и T%d", i+1, j+1)
			if canDeadlock(locks[i], locks[j]) {
				if correct != "" {
					return itembank.Instance{}, fmt.Errorf("seed %d: more than one deadlocking pair", seed)
				}
				correct = name
			} else {
				all = append(all, name)
			}
		}
	}
	if correct == "" {
		return itembank.Instance{}, fmt.Errorf("seed %d: no deadlocking pair", seed)
	}
	options, key := itembank.ShuffledOptions(r, correct, itembank.PickN(r, all, 3)...)
	a, b := locks[pair[0]], locks[pair[1]]
	stems := []string{
		"Четыре транзакции стартуют одновременно и выполняют свои команды по порядку; каждая команда захватывает блокировку строки до COMMIT. Какая пара транзакций может взаимно заблокироваться (deadlock)?",
		"Ниже четыре транзакции, которые выполняются параллельно в PostgreSQL. Между какими двумя из них возможна взаимная блокировка?",
		"Четыре сессии одновременно выполняют приведённые транзакции. Для какой пары сессий возможен deadlock?",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "sql", Source: strings.TrimRight(code.String(), "\n")},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: fmt.Sprintf("T%d захватывает строку %d и ждёт строку %d, а T%d захватывает %d и ждёт %d — образуется цикл ожидания. "+
			"Остальные транзакции берут блокировки в возрастающем порядке id или не пересекаются по обеим строкам, поэтому цикла с ними нет. "+
			"Защита — единый порядок захвата строк во всех транзакциях.",
			pair[0]+1, a[0], a[1], pair[1]+1, b[0], b[1]),
	}, nil
}

// isolationVisibleValue — какое значение увидит SELECT в T1 при заданном уровне изоляции.
type isolationVisibleValue struct{}

func (isolationVisibleValue) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.transactions.isolation-visible-value.v1", Spec: itembank.SpecBackend, Topic: "transactions",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.4, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"sql", "postgres", "mvcc"},
	}
}

func (f isolationVisibleValue) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x7A03)
	table := itembank.Ident(r, "table")
	col := itembank.Pick(r, []string{"balance", "qty", "score", "amount", "level"})
	id := itembank.IntBetween(r, 1, 50)
	v0 := itembank.IntBetween(r, 5, 50) * 10
	v1 := v0 + itembank.IntBetween(r, 1, 9)*10
	v2 := v1 + itembank.IntBetween(r, 1, 9)*10
	level := itembank.Pick(r, []string{"READ COMMITTED", "REPEATABLE READ", "SERIALIZABLE"})
	readCommitted := level == "READ COMMITTED"
	sel := fmt.Sprintf("SELECT %s FROM %s WHERE id = %d;", col, table, id)
	upd := func(v int) string { return fmt.Sprintf("UPDATE %s SET %s = %d WHERE id = %d;", table, col, v, id) }
	begin := fmt.Sprintf("BEGIN ISOLATION LEVEL %s;", level)
	variant := r.IntN(4)
	var rows [][]string
	var answer int
	var explain string
	switch variant {
	case 0:
		rows = [][]string{
			{"1", begin + " " + sel + " → " + strconv.Itoa(v0), ""},
			{"2", "", "BEGIN; " + upd(v1)},
			{"3", "", "COMMIT;"},
			{"4", sel + " → ?", ""},
		}
		if readCommitted {
			answer = v1
			explain = fmt.Sprintf("READ COMMITTED берёт новый снимок для каждого оператора, поэтому второй SELECT уже видит зафиксированное T2 значение %d.", v1)
		} else {
			answer = v0
			explain = fmt.Sprintf("На уровне %s снимок фиксируется при первом операторе транзакции; изменения, зафиксированные позже, не видны — T1 снова получает %d.", level, v0)
		}
	case 1:
		rows = [][]string{
			{"1", begin + " " + sel + " → " + strconv.Itoa(v0), ""},
			{"2", "", "BEGIN; " + upd(v1)},
			{"3", sel + " → ?", ""},
		}
		answer = v0
		explain = fmt.Sprintf("T2 ещё не зафиксировала изменение, а PostgreSQL не допускает грязного чтения ни на одном уровне изоляции, поэтому T1 видит прежнее значение %d.", v0)
	case 2:
		rows = [][]string{
			{"1", begin, ""},
			{"2", "", "BEGIN; " + upd(v1)},
			{"3", "", "COMMIT;"},
			{"4", sel + " → ?", ""},
		}
		answer = v1
		explain = fmt.Sprintf("Снимок берётся не в момент BEGIN, а при первом операторе транзакции. T2 зафиксировалась до первого SELECT в T1, поэтому даже на уровне %s виден %d.", level, v1)
	default:
		rows = [][]string{
			{"1", begin + " " + sel + " → " + strconv.Itoa(v0), ""},
			{"2", "", "BEGIN; " + upd(v1) + " COMMIT;"},
			{"3", "", "BEGIN; " + upd(v2)},
			{"4", sel + " → ?", ""},
		}
		if readCommitted {
			answer = v1
			explain = fmt.Sprintf("READ COMMITTED видит всё, что зафиксировано к началу оператора: первое обновление (%d) зафиксировано, второе (%d) — нет, поэтому результат %d.", v1, v2, v1)
		} else {
			answer = v0
			explain = fmt.Sprintf("На уровне %s T1 работает со снимком, сделанным первым SELECT, и ни зафиксированное %d, ни незафиксированное %d ей не видны — остаётся %d.", level, v1, v2, v0)
		}
	}
	stems := []string{
		fmt.Sprintf("PostgreSQL. Транзакция T1 работает на уровне изоляции `%s`, T2 — на уровне по умолчанию. Шаги выполняются строго в указанном порядке. Какое значение `%s` вернёт последний SELECT в T1?", level, col),
		fmt.Sprintf("Две сессии PostgreSQL выполняют команды в порядке шагов таблицы; T1 открыта на уровне `%s`. Что вернёт SELECT на последнем шаге?", level),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        itembank.Pick(r, stems) + "\n\n" + mdTable([]string{"Шаг", "T1", "T2"}, rows),
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: explain + " Типичная ошибка — путать момент снимка (BEGIN против первого оператора) и считать, что незафиксированные изменения видны.",
	}, nil
}

// nullAggregate — агрегат над столбцом с NULL.
type nullAggregate struct{}

func (nullAggregate) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.sql.null-aggregate.v1", Spec: itembank.SpecBackend, Topic: "sql",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: -0.7, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"sql", "null"},
	}
}

func (f nullAggregate) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5A13)
	table := itembank.Ident(r, "table")
	col := itembank.Pick(r, []string{"score", "rating", "qty", "weight", "age", "priority"})
	n := itembank.IntBetween(r, 5, 7)
	nulls := itembank.IntBetween(r, 1, 3)
	isNull := make([]bool, n)
	for _, i := range itembank.PickN(r, []int{0, 1, 2, 3, 4, 5, 6}[:n], nulls) {
		isNull[i] = true
	}
	var rows [][]string
	var vals []int
	distinct := map[int]bool{}
	for i := 0; i < n; i++ {
		cell := "NULL"
		if !isNull[i] {
			v := itembank.IntBetween(r, 1, 12)
			vals = append(vals, v)
			distinct[v] = true
			cell = strconv.Itoa(v)
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), cell})
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	agg := itembank.Pick(r, []string{"COUNT(*)", "COUNT(%s)", "SUM(%s)", "AVG(%s)", "COUNT(DISTINCT %s)"})
	expr := strings.ReplaceAll(agg, "%s", col)
	var answer, tol float64
	var explain string
	present := fmt.Sprintf("Значения без NULL: %s (их %d, сумма %d).", fmtInts(vals), len(vals), sum)
	switch agg {
	case "COUNT(*)":
		answer = float64(n)
		explain = fmt.Sprintf("COUNT(*) считает строки, а не значения, поэтому NULL тоже учитываются: %d строк. COUNT(%s) дал бы %d.", n, col, len(vals))
	case "COUNT(%s)":
		answer = float64(len(vals))
		explain = fmt.Sprintf("COUNT(столбец) пропускает NULL: из %d строк засчитаны %d. COUNT(*) дал бы %d.", n, len(vals), n)
	case "SUM(%s)":
		answer = float64(sum)
		explain = present + " SUM игнорирует NULL и не превращает результат в NULL, пока есть хотя бы одно значение."
	case "AVG(%s)":
		answer = float64(sum) / float64(len(vals))
		tol = 0.01
		explain = present + fmt.Sprintf(" AVG делит сумму только на число не-NULL значений: %d / %d = %.2f. Типичная ошибка — делить на общее число строк %d.", sum, len(vals), answer, n)
	default:
		answer = float64(len(distinct))
		explain = present + fmt.Sprintf(" COUNT(DISTINCT) пропускает NULL и считает различные значения: их %d.", len(distinct))
	}
	stems := []string{
		fmt.Sprintf("Таблица `%s` (NULL — отсутствие значения):\n\n%s\nЧто вернёт запрос? Для дробного результата укажите значение с точностью до сотых.", table, mdTable([]string{"id", col}, rows)),
		fmt.Sprintf("Содержимое таблицы `%s`:\n\n%s\nКакое число вернёт запрос ниже (дробное — с точностью до сотых)?", table, mdTable([]string{"id", col}, rows)),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        itembank.Pick(r, stems),
		Code:        &itembank.CodeBlock{Lang: "sql", Source: fmt.Sprintf("SELECT %s FROM %s;", expr, table)},
		Answer:      itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: explain,
	}, nil
}

// windowTopPerGroup — результат запроса с оконной функцией.
type windowTopPerGroup struct{}

func (windowTopPerGroup) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.sql.window-top-per-group.v1", Spec: itembank.SpecBackend, Topic: "sql",
		Grade: itembank.GradeSenior, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.4, B: 0.9, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"sql", "window"},
	}
}

func (f windowTopPerGroup) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5A14)
	table := itembank.Pick(r, []string{"orders", "payments", "invoices", "bookings", "sales", "shipments"})
	customers := itembank.Idents(r, "name_lat", 3)
	n := itembank.IntBetween(r, 7, 9)
	amountPool := make([]int, 0, 30)
	for i := 1; i <= 30; i++ {
		amountPool = append(amountPool, i*10)
	}
	amounts := itembank.PickN(r, amountPool, n)
	type row struct {
		id       int
		customer string
		amount   int
	}
	data := make([]row, 0, n)
	byCustomer := map[string][]row{}
	for i := 0; i < n; i++ {
		who := customers[i%3]
		if i >= 3 {
			who = itembank.Pick(r, customers)
		}
		rw := row{i + 1, who, amounts[i]}
		data = append(data, rw)
		byCustomer[who] = append(byCustomer[who], rw)
	}
	var md [][]string
	for _, d := range data {
		md = append(md, []string{strconv.Itoa(d.id), d.customer, strconv.Itoa(d.amount)})
	}
	var query, cols, explain string
	result := [][]string{}
	if r.IntN(2) == 0 {
		k := itembank.IntBetween(r, 1, 2)
		query = fmt.Sprintf("SELECT customer, amount\nFROM (\n  SELECT customer, amount,\n         ROW_NUMBER() OVER (PARTITION BY customer ORDER BY amount DESC) AS rn\n  FROM %s\n) s\nWHERE rn <= %d\nORDER BY customer, amount DESC;", table, k)
		cols = "customer, amount"
		names := append([]string{}, customers...)
		sort.Strings(names)
		var parts []string
		for _, who := range names {
			rs := byCustomer[who]
			sort.Slice(rs, func(i, j int) bool { return rs[i].amount > rs[j].amount })
			var top []string
			for i := 0; i < len(rs) && i < k; i++ {
				result = append(result, []string{who, strconv.Itoa(rs[i].amount)})
				top = append(top, strconv.Itoa(rs[i].amount))
			}
			parts = append(parts, who+": "+strings.Join(top, ", "))
		}
		explain = fmt.Sprintf("ROW_NUMBER нумерует строки внутри каждой группы customer по убыванию amount, внешний WHERE оставляет первые %d в каждой группе, "+
			"а ORDER BY упорядочивает по имени и затем по сумме. Отобранные значения — %s. Типичная ошибка — применить фильтр rn до группировки или перепутать порядок сортировки.", k, strings.Join(parts, "; "))
	} else {
		who := data[r.IntN(n)].customer
		query = fmt.Sprintf("SELECT id, amount,\n       SUM(amount) OVER (ORDER BY id) AS running\nFROM %s\nWHERE customer = '%s'\nORDER BY id;", table, who)
		cols = "id, amount, running"
		running := 0
		var parts []string
		for _, d := range byCustomer[who] {
			running += d.amount
			result = append(result, []string{strconv.Itoa(d.id), strconv.Itoa(d.amount), strconv.Itoa(running)})
			parts = append(parts, fmt.Sprintf("id %d → %d", d.id, running))
		}
		explain = fmt.Sprintf("WHERE отбирает строки клиента %s до вычисления окна, поэтому нарастающий итог считается только по его строкам в порядке id: %s. "+
			"Типичная ошибка — считать итог по всей таблице или по строкам без сортировки.", who, strings.Join(parts, ", "))
	}
	stems := []string{
		fmt.Sprintf("Таблица `%s`:\n\n%s\nКакие строки вернёт запрос (PostgreSQL)? Укажите все строки результата в порядке выдачи (%s).", table, mdTable([]string{"id", "customer", "amount"}, md), cols),
		fmt.Sprintf("Содержимое `%s`:\n\n%s\nПеречислите строки результата запроса в порядке выдачи (столбцы: %s).", table, mdTable([]string{"id", "customer", "amount"}, md), cols),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        itembank.Pick(r, stems),
		Code:        &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer:      itembank.Answer{Rows: result},
		Explanation: explain,
	}, nil
}

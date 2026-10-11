package common

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(sqlCountNull{})
	itembank.Register(sqlLeftJoinRows{})
	itembank.Register(sqlOrderLimit{})
	itembank.Register(sqlSubqueryIn{})
}

// mdTable рисует таблицу Markdown по заголовкам и строкам.
func mdTable(headers []string, rows [][]string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "| %s |\n|%s\n", strings.Join(headers, " | "), strings.Repeat("---|", len(headers)))
	for _, row := range rows {
		fmt.Fprintf(&sb, "| %s |\n", strings.Join(row, " | "))
	}
	return sb.String()
}

// fkName — имя внешнего ключа на таблицу: orders → order_id.
func fkName(table string) string { return strings.TrimSuffix(table, "s") + "_id" }

func sqlDir(desc bool) string {
	if desc {
		return "DESC"
	}
	return "ASC"
}

// sqlCountNull — семантика COUNT(*), COUNT(col), COUNT(DISTINCT col), сравнений и GROUP BY при NULL.
type sqlCountNull struct{}

func (sqlCountNull) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.sql.count-null.v1", Spec: itembank.SpecCommon, Topic: "sql",
		Grade: itembank.GradeJunior, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.1, B: -0.5, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"sql"},
	}
}

func (f sqlCountNull) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE16)
	itoa := strconv.Itoa
	table := itembank.Ident(r, "table")
	col := itembank.Pick(r, []string{"discount", "manager_id", "rating", "bonus", "parent_id", "category_id", "region_id"})
	n := itembank.IntBetween(r, 6, 9)
	pool := itembank.PickN(r, []int{1, 2, 3, 4, 5, 6, 7, 8, 9}, 3)
	nulls := itembank.IntBetween(r, 1, 3)
	vals := make([]int, n) // −1 — NULL
	for i := range vals {
		vals[i] = itembank.Pick(r, pool) * 10
	}
	for _, i := range r.Perm(n)[:nulls] {
		vals[i] = -1
	}
	filled := 0
	counts := map[int]int{}
	rows := make([][]string, n)
	for i, v := range vals {
		cell := "NULL"
		if v >= 0 {
			filled++
			counts[v]++
			cell = itoa(v)
		}
		rows[i] = []string{itoa(i + 1), cell}
	}
	present := make([]int, 0, len(counts))
	for v := range counts {
		present = append(present, v)
	}
	sort.Ints(present)

	var query, hint, explain string
	var result [][]string
	switch r.IntN(4) {
	case 0:
		query = fmt.Sprintf("SELECT COUNT(*), COUNT(%s), COUNT(DISTINCT %s)\nFROM %s;", col, col, table)
		result = [][]string{{itoa(n), itoa(filled), itoa(len(present))}}
		hint = " (одна строка из трёх значений)"
		explain = fmt.Sprintf("COUNT(*) считает все строки — %d; COUNT(%s) — только строки с не-NULL значением — %d; COUNT(DISTINCT %s) — различные "+
			"не-NULL значения — %d. Типичная ошибка — считать NULL отдельным значением или полагать, что COUNT(столбец) всегда равен COUNT(*).",
			n, col, filled, col, len(present))
	case 1:
		query = fmt.Sprintf("SELECT COUNT(*) AS total, COUNT(%s) AS filled\nFROM %s;", col, table)
		result = [][]string{{itoa(n), itoa(filled)}}
		hint = " (одна строка из двух значений)"
		explain = fmt.Sprintf("COUNT(*) считает все строки — %d, а COUNT(%s) пропускает NULL — %d (строк с NULL здесь %d). Типичная ошибка — считать, "+
			"что оба выражения всегда дают одно и то же.", n, col, filled, nulls)
	case 2:
		x := itembank.Pick(r, present)
		cnt := filled - counts[x]
		query = fmt.Sprintf("SELECT COUNT(*)\nFROM %s\nWHERE %s <> %d;", table, col, x)
		result = [][]string{{itoa(cnt)}}
		hint = " (одна строка из одного значения)"
		explain = fmt.Sprintf("Сравнение NULL <> %d даёт UNKNOWN, и WHERE такие строки не пропускает: из %d строк остаются только не-NULL и не равные %d — %d. "+
			"Типичная ошибка — включать строки с NULL, считая, что NULL «не равен» %d.", x, n, x, cnt, x)
	default:
		query = fmt.Sprintf("SELECT %s, COUNT(*) AS cnt\nFROM %s\nGROUP BY %s\nORDER BY %s ASC NULLS LAST;", col, table, col, col)
		for _, v := range present {
			result = append(result, []string{itoa(v), itoa(counts[v])})
		}
		result = append(result, []string{"NULL", itoa(nulls)})
		hint = " в порядке выдачи; NULL записывайте как `NULL`"
		explain = fmt.Sprintf("GROUP BY объединяет все NULL в одну группу (%d строк), остальные группы — по значениям; NULLS LAST ставит группу NULL "+
			"последней. Типичная ошибка — терять группу NULL, ставить её первой или разбивать NULL на отдельные группы.", nulls)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Таблица `%s` (NULL — отсутствующее значение):\n\n%s\nЧто вернёт запрос? Укажите строки результата%s.",
			table, mdTable([]string{"id", col}, rows), hint),
		Code:        &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer:      itembank.Answer{Rows: result},
		Explanation: explain,
	}, nil
}

// sqlLeftJoinRows — сколько строк даёт LEFT JOIN при повторяющихся и отсутствующих ключах.
type sqlLeftJoinRows struct{}

func (sqlLeftJoinRows) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.sql.left-join-rows.v1", Spec: itembank.SpecCommon, Topic: "sql",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.6, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"sql"},
	}
}

func (f sqlLeftJoinRows) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE17)
	names := itembank.Idents(r, "table", 2)
	left, right := names[0], names[1]
	fk := fkName(left)
	nl := itembank.IntBetween(r, 4, 6)
	leftIDs := make([]int, nl)
	for i := range leftIDs {
		leftIDs[i] = i + 1
	}
	rightFK := make([]int, itembank.IntBetween(r, 4, 7))
	for i := range rightFK {
		rightFK[i] = itembank.IntBetween(r, 1, nl+2) // часть ссылок — на несуществующие id
	}
	total, unmatched := 0, 0
	var perRow, orphans []string
	for _, id := range leftIDs {
		m := 0
		for _, k := range rightFK {
			if k == id {
				m++
			}
		}
		perRow = append(perRow, fmt.Sprintf("%d → %d", id, m))
		if m == 0 {
			unmatched++
			total++
			continue
		}
		total += m
	}
	for _, k := range rightFK {
		if k > nl && !slices.Contains(orphans, strconv.Itoa(k)) {
			orphans = append(orphans, strconv.Itoa(k))
		}
	}
	orphanNote := ""
	if len(orphans) > 0 {
		orphanNote = fmt.Sprintf(" Строки правой таблицы со значениями %s не имеют пары слева и в результат не попадают.", strings.Join(orphans, ", "))
	}

	var query, explain string
	var answer int
	if r.IntN(2) == 0 {
		query = fmt.Sprintf("SELECT *\nFROM %s a\nLEFT JOIN %s b ON b.%s = a.id;", left, right, fk)
		answer = total
		explain = fmt.Sprintf("LEFT JOIN сохраняет каждую строку левой таблицы: при m совпадениях справа она даёт m строк, без совпадений — одну строку "+
			"с NULL в столбцах правой. Совпадения по id: %s; итого %d.%s Типичная ошибка — считать как INNER JOIN (терять строки без пары) или "+
			"добавлять строки правой таблицы без пары.", strings.Join(perRow, ", "), total, orphanNote)
	} else {
		query = fmt.Sprintf("SELECT a.id\nFROM %s a\nLEFT JOIN %s b ON b.%s = a.id\nWHERE b.%s IS NULL;", left, right, fk, fk)
		answer = unmatched
		explain = fmt.Sprintf("Условие `b.%s IS NULL` после LEFT JOIN оставляет только строки левой таблицы, для которых пары не нашлось. Совпадения по id: %s; "+
			"без пары — %d.%s Типичная ошибка — считать строки правой таблицы без пары или строки с одним совпадением.",
			fk, strings.Join(perRow, ", "), unmatched, orphanNote)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("В таблице `%s` столбец `id` (первичный ключ) содержит значения (%s), в таблице `%s` столбец `%s` — (%s). Сколько строк вернёт запрос?",
			left, joinInts(leftIDs, ", "), right, fk, joinInts(rightFK, ", ")),
		Code:        &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: explain,
	}, nil
}

// sqlOrderLimit — результат ORDER BY с вторичным ключом, LIMIT и OFFSET.
type sqlOrderLimit struct{}

func (sqlOrderLimit) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.sql.order-limit-offset.v1", Spec: itembank.SpecCommon, Topic: "sql",
		Grade: itembank.GradeJunior, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.1, B: -0.4, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"sql"},
	}
}

func (f sqlOrderLimit) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE18)
	table := itembank.Ident(r, "table")
	col := itembank.Pick(r, []string{"score", "amount", "rating", "total", "age"})
	n := itembank.IntBetween(r, 5, 7)
	names := itembank.Idents(r, "name_lat", n)
	type row struct {
		name string
		val  int
	}
	data := make([]row, n)
	withTies := r.IntN(2) == 0
	if withTies {
		pool := itembank.PickN(r, []int{1, 2, 3, 4, 5, 6, 7, 8, 9}, 3)
		for i := range data {
			data[i] = row{names[i], itembank.Pick(r, pool) * 10}
		}
	} else {
		vals := distinctInts(r, n, 10, 99)
		for i := range data {
			data[i] = row{names[i], vals[i]}
		}
	}
	desc := r.IntN(2) == 0
	nameDesc := withTies && r.IntN(2) == 0
	sorted := slices.Clone(data)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.val != b.val {
			if desc {
				return a.val > b.val
			}
			return a.val < b.val
		}
		if nameDesc {
			return a.name > b.name
		}
		return a.name < b.name
	})
	limit := itembank.IntBetween(r, 2, 3)
	offset := itembank.IntBetween(r, 0, n-2)
	picked := sorted[offset:min(offset+limit, n)]

	result := make([][]string, 0, len(picked))
	for _, p := range picked {
		result = append(result, []string{p.name, strconv.Itoa(p.val)})
	}
	shown := make([][]string, n)
	for i, d := range data {
		shown[i] = []string{d.name, strconv.Itoa(d.val)}
	}
	sortedText := make([]string, n)
	for i, p := range sorted {
		sortedText[i] = fmt.Sprintf("%s (%d)", p.name, p.val)
	}
	orderBy := fmt.Sprintf("%s %s", col, sqlDir(desc))
	tieNote := ""
	if withTies {
		orderBy += fmt.Sprintf(", name %s", sqlDir(nameDesc))
		tieNote = fmt.Sprintf(" При равных %s порядок определяет второй ключ — name %s.", col, sqlDir(nameDesc))
	}
	query := fmt.Sprintf("SELECT name, %s\nFROM %s\nORDER BY %s\nLIMIT %d OFFSET %d;", col, table, orderBy, limit, offset)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Таблица `%s`:\n\n%s\nКакие строки вернёт запрос? Укажите строки результата (name, %s) в порядке выдачи.",
			table, mdTable([]string{"name", col}, shown), col),
		Code:   &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer: itembank.Answer{Rows: result},
		Explanation: fmt.Sprintf("После сортировки порядок строк: %s.%s OFFSET %d пропускает первые %d строк, LIMIT %d берёт следующие не более %d — "+
			"в результате %d. Типичная ошибка — применять LIMIT до OFFSET, пропускать на одну строку больше или игнорировать второй ключ сортировки.",
			strings.Join(sortedText, ", "), tieNote, offset, offset, limit, limit, len(result)),
	}, nil
}

// sqlSubqueryIn — IN / NOT IN с подзапросом, в том числе ловушка NOT IN при NULL в подзапросе.
type sqlSubqueryIn struct{}

func (sqlSubqueryIn) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.sql.subquery-in.v1", Spec: itembank.SpecCommon, Topic: "sql",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.3, B: 0.4, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"sql"},
	}
}

func (f sqlSubqueryIn) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE19)
	left := itembank.Pick(r, []string{"customers", "users", "employees", "accounts", "clients"})
	right := itembank.Pick(r, []string{"orders", "payments", "tickets", "bookings", "invoices"})
	fk := fkName(left)
	nl := itembank.IntBetween(r, 4, 5)
	names := itembank.Idents(r, "name_lat", nl)
	nr := itembank.IntBetween(r, 5, 7)
	notIn := r.IntN(2) == 0
	withNull := r.IntN(3) == 0
	threshold := itembank.IntBetween(r, 3, 9) * 10

	type order struct{ fk, amount int } // fk == 0 — NULL
	orders := make([]order, nr)
	for i := range orders {
		orders[i] = order{itembank.IntBetween(r, 1, nl), itembank.IntBetween(r, 1, 12) * 10}
	}
	if withNull {
		// Строка с NULL в fk заведомо проходит фильтр по сумме, чтобы NULL попал в результат подзапроса.
		orders[r.IntN(nr)] = order{0, threshold + itembank.IntBetween(r, 1, 3)*10}
	}
	sub := map[int]bool{}
	var subList []string
	nullInSub := false
	for _, o := range orders {
		if o.amount <= threshold {
			continue
		}
		if o.fk == 0 {
			nullInSub = true
			subList = append(subList, "NULL")
			continue
		}
		sub[o.fk] = true
		subList = append(subList, strconv.Itoa(o.fk))
	}
	if len(subList) == 0 {
		subList = []string{"пусто"}
	}

	leftRows := make([][]string, nl)
	idx := make([]int, nl)
	for i := range names {
		leftRows[i] = []string{strconv.Itoa(i + 1), names[i]}
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return names[idx[a]] < names[idx[b]] })
	result := [][]string{}
	for _, i := range idx {
		in := sub[i+1]
		include := in
		if notIn {
			include = !nullInSub && !in
		}
		if include {
			result = append(result, []string{names[i]})
		}
	}
	rightRows := make([][]string, nr)
	for i, o := range orders {
		cell := "NULL"
		if o.fk > 0 {
			cell = strconv.Itoa(o.fk)
		}
		rightRows[i] = []string{strconv.Itoa(i + 1), cell, strconv.Itoa(o.amount)}
	}

	op := "IN"
	explain := fmt.Sprintf("Подзапрос возвращает значения %s с amount > %d: (%s). IN проверяет вхождение id в этот список; NULL в списке на результат IN "+
		"не влияет, строки без подходящих заказов не проходят. Типичная ошибка — брать всех, у кого есть хоть один заказ, забывая про фильтр по сумме.",
		fk, threshold, strings.Join(subList, ", "))
	switch {
	case notIn && nullInSub:
		op = "NOT IN"
		explain = fmt.Sprintf("Подзапрос возвращает (%s) — в списке есть NULL. NOT IN раскрывается в `id <> v₁ AND id <> v₂ AND id <> NULL`; сравнение с NULL "+
			"даёт UNKNOWN, конъюнкция не становится TRUE, и ни одна строка не проходит — результат пуст. Типичная ошибка — считать, что NULL просто "+
			"игнорируется, как в IN; безопасная замена — NOT EXISTS или `WHERE %s IS NOT NULL` в подзапросе.", strings.Join(subList, ", "), fk)
	case notIn:
		op = "NOT IN"
		explain = fmt.Sprintf("Подзапрос возвращает значения %s с amount > %d: (%s); NULL среди них нет, поэтому NOT IN работает как ожидается и оставляет "+
			"строки, id которых нет в списке. Типичная ошибка — забыть про фильтр по сумме в подзапросе или перепутать направление отбора.",
			fk, threshold, strings.Join(subList, ", "))
	}
	query := fmt.Sprintf("SELECT name\nFROM %s\nWHERE id %s (\n  SELECT %s FROM %s WHERE amount > %d\n)\nORDER BY name;", left, op, fk, right, threshold)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Таблицы `%s` и `%s` (NULL — отсутствующее значение):\n\n%s\n%s\nКакие строки вернёт запрос? Укажите строки результата (name) "+
			"в порядке выдачи; если результат пуст — оставьте список строк пустым.",
			left, right, mdTable([]string{"id", "name"}, leftRows), mdTable([]string{"id", fk, "amount"}, rightRows)),
		Code:        &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer:      itembank.Answer{Rows: result},
		Explanation: explain,
	}, nil
}

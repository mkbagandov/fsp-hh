package dataml

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(groupbyAgg{})
	itembank.Register(mergeRows{})
	itembank.Register(leftJoinCount{})
	itembank.Register(windowRank{})
}

// round2 округляет до сотых.
func round2(x float64) float64 { return math.Round(x*100) / 100 }

// groupbyAgg — значение агрегата groupby для одной группы маленького DataFrame.
type groupbyAgg struct{}

func (groupbyAgg) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.pandas_sql.groupby-agg.v1", Spec: itembank.SpecDataML, Topic: "pandas_sql",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.0, B: -0.9, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"pandas", "python"},
	}
}

func (f groupbyAgg) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A20)
	keyCol := itembank.Pick(r, []string{"city", "branch", "office"})
	valCol := itembank.Pick(r, []string{"amount", "price", "qty", "score", "duration"})
	groups := itembank.Idents(r, "city", 3)
	n := itembank.IntBetween(r, 6, 8)
	keys := make([]string, n)
	vals := make([]int, n)
	for i := range keys {
		keys[i] = groups[i%3]
		if i >= 3 {
			keys[i] = itembank.Pick(r, groups)
		}
		vals[i] = itembank.IntBetween(r, 2, 40) * 5
	}
	perm := r.Perm(n)
	shKeys := make([]string, n)
	shVals := make([]int, n)
	for i, p := range perm {
		shKeys[i], shVals[i] = keys[p], vals[p]
	}
	// Группа с не менее чем двумя строками есть всегда (n ≥ 6 при трёх группах).
	byGroup := map[string][]int{}
	for i, k := range shKeys {
		byGroup[k] = append(byGroup[k], shVals[i])
	}
	candidates := make([]string, 0, 3)
	for _, g := range groups {
		if len(byGroup[g]) >= 2 {
			candidates = append(candidates, g)
		}
	}
	target := itembank.Pick(r, candidates)
	members := byGroup[target]
	sorted := append([]int{}, members...)
	sort.Ints(sorted)
	sum := sumInts(members)
	agg := itembank.Pick(r, []string{"mean", "sum", "count", "median", "max"})
	var answer, tol float64
	var explain string
	switch agg {
	case "mean":
		answer, tol = round2(float64(sum)/float64(len(members))), 0.011
		explain = fmt.Sprintf("mean — сумма значений группы %d, делённая на число её строк %d: %.2f.", sum, len(members), float64(sum)/float64(len(members)))
	case "sum":
		answer = float64(sum)
		explain = fmt.Sprintf("sum — сумма значений группы: %d.", sum)
	case "count":
		answer = float64(len(members))
		explain = fmt.Sprintf("count — число строк группы: %d.", len(members))
	case "median":
		m := len(sorted)
		med := float64(sorted[m/2])
		if m%2 == 0 {
			med = float64(sorted[m/2-1]+sorted[m/2]) / 2
		}
		answer, tol = round2(med), 0.011
		explain = fmt.Sprintf("median — середина отсортированных значений группы %v: %.2f (при чётном числе — среднее двух центральных).", sorted, med)
	default:
		answer = float64(sorted[len(sorted)-1])
		explain = fmt.Sprintf("max — наибольшее значение группы: %d.", sorted[len(sorted)-1])
	}
	code := fmt.Sprintf("import pandas as pd\n\ndf = pd.DataFrame({\n    \"%s\": %s,\n    \"%s\": %s,\n})\nres = df.groupby(\"%s\")[\"%s\"].%s()",
		keyCol, pyStrList(shKeys), valCol, pyIntList(shVals), keyCol, valCol, agg)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   fmt.Sprintf("Чему равно `res[\"%s\"]`? При необходимости округлите до сотых.", target),
		Code:   &itembank.CodeBlock{Lang: "python", Source: code},
		Answer: itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: fmt.Sprintf("groupby разбивает строки по значению `%s`; в группу «%s» попадают значения %s. %s "+
			"Типичная ошибка — учитывать строки других групп или делить на общее число строк таблицы.", keyCol, target, pyIntList(members), explain),
	}, nil
}

// mergeRows — число строк после pd.merge с повторяющимися ключами и разными how.
type mergeRows struct{}

func (mergeRows) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.pandas_sql.merge-rows.v1", Spec: itembank.SpecDataML, Topic: "pandas_sql",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.1, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"pandas", "python"},
	}
}

var mergePairs = [][3]string{
	{"orders", "clients", "client_id"}, {"events", "users", "user_id"},
	{"tasks", "employees", "employee_id"}, {"payments", "invoices", "invoice_id"},
}

// mergedRows — сколько строк даёт ключ с m вхождениями слева и n справа при данном how.
func mergedRows(m, n int, how string) int {
	switch {
	case m > 0 && n > 0:
		return m * n
	case how == "left" && m > 0, how == "right" && n > 0, how == "outer":
		return m + n
	}
	return 0
}

func (f mergeRows) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A21)
	p := itembank.Pick(r, mergePairs)
	how := itembank.Pick(r, []string{"inner", "left", "right", "outer"})
	leftKeys := make([]int, itembank.IntBetween(r, 4, 6))
	rightKeys := make([]int, itembank.IntBetween(r, 3, 5))
	leftVals := make([]int, len(leftKeys))
	rightVals := make([]string, len(rightKeys))
	for i := range leftKeys {
		leftKeys[i] = itembank.IntBetween(r, 1, 5)
		leftVals[i] = itembank.IntBetween(r, 1, 20) * 10
	}
	for i := range rightKeys {
		rightKeys[i] = itembank.IntBetween(r, 1, 5)
		rightVals[i] = itembank.Ident(r, "city")
	}
	mL, mR := map[int]int{}, map[int]int{}
	for _, k := range leftKeys {
		mL[k]++
	}
	for _, k := range rightKeys {
		mR[k]++
	}
	total := 0
	var parts []string
	for k := 1; k <= 5; k++ {
		if mL[k] == 0 && mR[k] == 0 {
			continue
		}
		c := mergedRows(mL[k], mR[k], how)
		total += c
		parts = append(parts, fmt.Sprintf("ключ %d: %d×%d → %d", k, mL[k], mR[k], c))
	}
	code := fmt.Sprintf("import pandas as pd\n\n%s = pd.DataFrame({\"%s\": %s, \"amount\": %s})\n%s = pd.DataFrame({\"%s\": %s, \"city\": %s})\nres = %s.merge(%s, on=\"%s\", how=\"%s\")\nprint(len(res))",
		p[0], p[2], pyIntList(leftKeys), pyIntList(leftVals), p[1], p[2], pyIntList(rightKeys), pyStrList(rightVals), p[0], p[1], p[2], how)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   "Какое число выведет код?",
		Code:   &itembank.CodeBlock{Lang: "python", Source: code},
		Answer: itembank.Answer{Number: float64(total)},
		Explanation: fmt.Sprintf("Ключ, встречающийся m раз слева и n раз справа, даёт m·n строк; при how=\"%s\" ключи без пары %s. По ключам: %s; итого %d. "+
			"Типичная ошибка — считать, что merge сохраняет число строк левой таблицы, или забывать про размножение строк при дублях ключа с обеих сторон.",
			how, mergeNoPairNote(how), strings.Join(parts, "; "), total),
	}, nil
}

func mergeNoPairNote(how string) string {
	switch how {
	case "inner":
		return "отбрасываются"
	case "left":
		return "сохраняются только из левой таблицы (по одной строке на вхождение, справа NaN)"
	case "right":
		return "сохраняются только из правой таблицы (по одной строке на вхождение, слева NaN)"
	}
	return "сохраняются из обеих таблиц (по одной строке на вхождение, недостающие поля — NaN)"
}

// leftJoinCount — COUNT после LEFT JOIN: нули для строк без пары, COUNT(*) против COUNT(col),
// условие в ON.
type leftJoinCount struct{}

func (leftJoinCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.pandas_sql.left-join-count.v1", Spec: itembank.SpecDataML, Topic: "pandas_sql",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.2, B: 0.3, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"sql"},
	}
}

var joinPairs = [][3]string{
	{"customers", "orders", "customer_id"}, {"users", "sessions", "user_id"},
	{"employees", "tasks", "employee_id"}, {"authors", "posts", "author_id"},
}

func (f leftJoinCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A22)
	p := itembank.Pick(r, joinPairs)
	parent, child, fk := p[0], p[1], p[2]
	nParents := itembank.IntBetween(r, 3, 4)
	names := itembank.Idents(r, "name_lat", nParents)
	nChildren := itembank.IntBetween(r, 4, 7)
	// Хотя бы один родитель без дочерних строк: именно он отличает LEFT JOIN от INNER.
	lonely := 1 + r.IntN(nParents)
	type childRow struct{ id, parentID, amount int }
	children := make([]childRow, 0, nChildren)
	for i := 0; i < nChildren; i++ {
		pid := 1 + r.IntN(nParents)
		if pid == lonely {
			pid = lonely%nParents + 1
		}
		children = append(children, childRow{id: 100 + i + 1, parentID: pid, amount: itembank.IntBetween(r, 1, 12) * 10})
	}
	orphan := r.IntN(2) == 0
	if orphan {
		children = append(children, childRow{id: 100 + nChildren + 1, parentID: nParents + 5, amount: itembank.IntBetween(r, 1, 12) * 10})
	}
	variant := r.IntN(3)
	threshold := itembank.IntBetween(r, 3, 9) * 10
	var countExpr, onExtra, explainVariant string
	switch variant {
	case 0:
		countExpr = "COUNT(o.id)"
		explainVariant = "COUNT(o.id) не учитывает NULL, поэтому у строк без пары счётчик равен 0"
	case 1:
		countExpr = "COUNT(*)"
		explainVariant = "COUNT(*) считает строки результата соединения: у родителя без пары остаётся одна строка с NULL справа, и счётчик равен 1, а не 0"
	default:
		countExpr = "COUNT(o.id)"
		onExtra = fmt.Sprintf(" AND o.amount > %d", threshold)
		explainVariant = fmt.Sprintf("условие `o.amount > %d` стоит в ON, поэтому отфильтровываются только дочерние строки, а родители остаются с нулём; "+
			"в WHERE оно отбросило бы и родителей без пары", threshold)
	}
	var md strings.Builder
	fmt.Fprintf(&md, "`%s`:\n\n| id | name |\n|---|---|\n", parent)
	for i, nm := range names {
		fmt.Fprintf(&md, "| %d | %s |\n", i+1, nm)
	}
	fmt.Fprintf(&md, "\n`%s`:\n\n| id | %s | amount |\n|---|---|---|\n", child, fk)
	for _, c := range children {
		fmt.Fprintf(&md, "| %d | %d | %d |\n", c.id, c.parentID, c.amount)
	}
	result := make([][]string, 0, nParents)
	order := append([]string{}, names...)
	sort.Strings(order)
	for _, nm := range order {
		pid := 0
		for i, x := range names {
			if x == nm {
				pid = i + 1
			}
		}
		matched := 0
		for _, c := range children {
			if c.parentID == pid && (variant != 2 || c.amount > threshold) {
				matched++
			}
		}
		if variant == 1 && matched == 0 {
			matched = 1
		}
		result = append(result, []string{nm, strconv.Itoa(matched)})
	}
	query := fmt.Sprintf("SELECT c.name, %s AS cnt\nFROM %s c\nLEFT JOIN %s o ON o.%s = c.id%s\nGROUP BY c.name\nORDER BY c.name;",
		countExpr, parent, child, fk, onExtra)
	orphanNote := ""
	if orphan {
		orphanNote = fmt.Sprintf(" Строка `%s` с %s = %d не имеет родителя и в LEFT JOIN от `%s` не попадает.", child, fk, nParents+5, parent)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   fmt.Sprintf("Даны таблицы.\n\n%s\nКакие строки вернёт запрос? Укажите все строки результата в порядке выдачи (name, cnt).", md.String()),
		Code:   &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer: itembank.Answer{Rows: result},
		Explanation: fmt.Sprintf("LEFT JOIN сохраняет каждую строку `%s`, даже если в `%s` нет совпадений; %s.%s "+
			"Типичная ошибка — терять родителей без дочерних строк (как в INNER JOIN) или путать COUNT(*) и COUNT(столбец).",
			parent, child, explainVariant, orphanNote),
	}, nil
}

// windowRank — RANK / DENSE_RANK / ROW_NUMBER по разделам с дублями значений.
type windowRank struct{}

func (windowRank) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.pandas_sql.window-rank.v1", Spec: itembank.SpecDataML, Topic: "pandas_sql",
		Grade: itembank.GradeSenior, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.4, B: 0.9, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"sql", "window-functions"},
	}
}

type saleRow struct {
	id     int
	dept   string
	name   string
	amount int
}

func (f windowRank) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A23)
	table := itembank.Pick(r, []string{"sales", "bonuses", "deals", "payouts"})
	depts := itembank.PickN(r, []string{"sales", "support", "dev", "ops", "hr", "marketing"}, itembank.IntBetween(r, 2, 3))
	n := itembank.IntBetween(r, 6, 8)
	names := itembank.Idents(r, "name_lat", n)
	rows := make([]saleRow, n)
	for i := range rows {
		dept := depts[i%len(depts)]
		if i >= len(depts) {
			dept = itembank.Pick(r, depts)
		}
		rows[i] = saleRow{id: i + 1, dept: dept, name: names[i], amount: itembank.IntBetween(r, 5, 30) * 10}
	}
	// В большинстве экземпляров внутри одного раздела есть равные значения — именно на них
	// RANK, DENSE_RANK и ROW_NUMBER расходятся.
	if r.IntN(10) < 7 {
		i := r.IntN(n)
		for j := range rows {
			if j != i && rows[j].dept == rows[i].dept {
				rows[j].amount = rows[i].amount
				break
			}
		}
	}
	fn := itembank.Pick(r, []string{"RANK", "DENSE_RANK", "ROW_NUMBER"})
	desc := r.IntN(3) != 0
	limit := itembank.IntBetween(r, 1, 2)
	better := func(a, b saleRow) bool {
		if desc {
			return a.amount > b.amount
		}
		return a.amount < b.amount
	}
	result := [][]string{}
	for _, row := range rows {
		rk := 0
		switch fn {
		case "RANK":
			rk = 1
			for _, o := range rows {
				if o.dept == row.dept && better(o, row) {
					rk++
				}
			}
		case "DENSE_RANK":
			distinct := map[int]bool{}
			for _, o := range rows {
				if o.dept == row.dept && better(o, row) {
					distinct[o.amount] = true
				}
			}
			rk = 1 + len(distinct)
		default:
			rk = 1
			for _, o := range rows {
				if o.dept == row.dept && (better(o, row) || (o.amount == row.amount && o.id < row.id)) {
					rk++
				}
			}
		}
		if rk <= limit {
			result = append(result, []string{row.dept, row.name, strconv.Itoa(row.amount)})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i][0] != result[j][0] {
			return result[i][0] < result[j][0]
		}
		return result[i][1] < result[j][1]
	})
	dir := "DESC"
	if !desc {
		dir = "ASC"
	}
	orderExpr := "amount " + dir
	if fn == "ROW_NUMBER" {
		orderExpr += ", id"
	}
	cond := "rk = 1"
	if limit == 2 {
		cond = "rk <= 2"
	}
	query := fmt.Sprintf("SELECT dept, name, amount\nFROM (\n  SELECT dept, name, amount,\n         %s() OVER (PARTITION BY dept ORDER BY %s) AS rk\n  FROM %s\n) t\nWHERE %s\nORDER BY dept, name;",
		fn, orderExpr, table, cond)
	var md strings.Builder
	md.WriteString("| id | dept | name | amount |\n|---|---|---|---|\n")
	for _, row := range rows {
		fmt.Fprintf(&md, "| %d | %s | %s | %d |\n", row.id, row.dept, row.name, row.amount)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Таблица `%s`:\n\n%s\nКакие строки вернёт запрос? Укажите все строки результата в порядке выдачи (dept, name, amount).",
			table, md.String()),
		Code:   &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer: itembank.Answer{Rows: result},
		Explanation: fmt.Sprintf("Нумерация идёт отдельно в каждом разделе dept по %s. %s Условие `%s` оставляет %d строк, затем они сортируются по dept и name. "+
			"Типичная ошибка — считать ранги по всей таблице или не учитывать, как выбранная функция обрабатывает равные значения.",
			orderExpr, rankNote(fn), cond, len(result)),
	}, nil
}

func rankNote(fn string) string {
	switch fn {
	case "RANK":
		return "RANK даёт равным значениям одинаковый ранг и пропускает следующие номера (1, 1, 3)."
	case "DENSE_RANK":
		return "DENSE_RANK даёт равным значениям одинаковый ранг без пропусков (1, 1, 2)."
	}
	return "ROW_NUMBER нумерует строки подряд без повторов; при равных amount порядок задаёт id."
}

// Package backend — семейства заданий для backend-разработки: SQL, транзакции,
// HTTP/REST, конкурентность, кэширование, безопасность, тестирование, проектирование.
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
	itembank.Register(sqlJoinCount{})
	itembank.Register(sqlGroupHaving{})
}

// sqlJoinCount — сколько строк вернёт INNER JOIN при повторяющихся ключах.
type sqlJoinCount struct{}

func (sqlJoinCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.sql.join-count.v1", Spec: itembank.SpecBackend, Topic: "sql",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.6, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"sql", "postgres"},
	}
}

func fmtInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func (f sqlJoinCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5A11)
	names := itembank.Idents(r, "table", 2)
	left, right := names[0], names[1]
	key := itembank.Pick(r, []string{"id", "code", "ref", "key"})
	leftKeys := make([]int, itembank.IntBetween(r, 3, 5))
	rightKeys := make([]int, itembank.IntBetween(r, 3, 5))
	for i := range leftKeys {
		leftKeys[i] = itembank.IntBetween(r, 1, 4)
	}
	for i := range rightKeys {
		rightKeys[i] = itembank.IntBetween(r, 1, 4)
	}
	count := 0
	for _, a := range leftKeys {
		for _, b := range rightKeys {
			if a == b {
				count++
			}
		}
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("В таблице `%s` столбец `%s` содержит значения %s, в таблице `%s` столбец `%s` — %s. Сколько строк вернёт запрос?",
			left, key, fmtInts(leftKeys), right, key, fmtInts(rightKeys)),
		Code:   &itembank.CodeBlock{Lang: "sql", Source: fmt.Sprintf("SELECT *\nFROM %s a\nJOIN %s b ON a.%s = b.%s;", left, right, key, key)},
		Answer: itembank.Answer{Number: float64(count)},
		Explanation: fmt.Sprintf("INNER JOIN даёт по строке на каждую пару совпадающих ключей: значение, встречающееся m раз слева и n раз справа, "+
			"порождает m·n строк. Здесь получается %d. Типичная ошибка — считать число различных совпадающих значений или минимум из двух таблиц.", count),
	}, nil
}

// sqlGroupHaving — результат GROUP BY ... HAVING над маленькой таблицей.
type sqlGroupHaving struct{}

func (sqlGroupHaving) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.sql.group-having.v1", Spec: itembank.SpecBackend, Topic: "sql",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.3, B: 0.1, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"sql"},
	}
}

func (f sqlGroupHaving) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5A12)
	table := itembank.Ident(r, "table")
	customers := itembank.Idents(r, "name_lat", itembank.IntBetween(r, 3, 4))
	rows := itembank.IntBetween(r, 6, 8)
	type row struct {
		who    string
		amount int
	}
	data := make([]row, 0, rows)
	sums := map[string]int{}
	for i := 0; i < rows; i++ {
		who := itembank.Pick(r, customers)
		amt := itembank.IntBetween(r, 1, 12) * 10
		data = append(data, row{who, amt})
		sums[who] += amt
	}
	threshold := itembank.IntBetween(r, 5, 20) * 10
	var md strings.Builder
	md.WriteString("| customer | amount |\n|---|---|\n")
	for _, d := range data {
		fmt.Fprintf(&md, "| %s | %d |\n", d.who, d.amount)
	}
	var result [][]string
	names := make([]string, 0, len(sums))
	for who := range sums {
		names = append(names, who)
	}
	sort.Strings(names)
	for _, who := range names {
		if sums[who] > threshold {
			result = append(result, []string{who, strconv.Itoa(sums[who])})
		}
	}
	if result == nil {
		result = [][]string{}
	}
	query := fmt.Sprintf("SELECT customer, SUM(amount) AS total\nFROM %s\nGROUP BY customer\nHAVING SUM(amount) > %d\nORDER BY customer;", table, threshold)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Таблица `%s`:\n\n%s\nКакие строки вернёт запрос? Укажите все строки результата в порядке выдачи (customer, total); "+
			"если результат пуст — оставьте список строк пустым.", table, md.String()),
		Code:   &itembank.CodeBlock{Lang: "sql", Source: query},
		Answer: itembank.Answer{Rows: result},
		Explanation: fmt.Sprintf("GROUP BY суммирует amount по каждому customer, HAVING отбирает группы с суммой строго больше %d, ORDER BY упорядочивает по имени. "+
			"Суммы по группам: %s. Типичная ошибка — применять условие к отдельным строкам (как WHERE), а не к агрегату.", threshold, describeSums(names, sums)),
	}, nil
}

func describeSums(names []string, sums map[string]int) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s = %d", n, sums[n]))
	}
	return strings.Join(parts, ", ")
}

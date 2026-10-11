package dataml

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
	itembank.Register(oneHotShape{})
	itembank.Register(scalerTransform{})
	itembank.Register(targetEncoding{})
	itembank.Register(pivotTable{})
}

// catColumn — категориальный столбец: имя и словарь значений.
type catColumn struct {
	name   string
	values []string
}

var catColumns = []catColumn{
	{"plan", []string{"free", "basic", "pro", "team"}},
	{"device", []string{"ios", "android", "web", "desktop"}},
	{"segment", []string{"new", "returning", "vip", "churned"}},
	{"channel", []string{"email", "sms", "push", "ads"}},
	{"region", []string{"north", "south", "east", "west"}},
	{"tier", []string{"bronze", "silver", "gold"}},
}

var numColumns = []string{"age", "amount", "visits", "qty", "tenure", "score"}

// catSeries строит столбец из n строк ровно с k различными значениями из словаря pool.
func catSeries(r *rand.Rand, pool []string, n, k int) []string {
	vals := itembank.PickN(r, pool, k)
	col := make([]string, n)
	for i := range col {
		if i < k {
			col[i] = vals[i]
			continue
		}
		col[i] = itembank.Pick(r, vals)
	}
	return itembank.Shuffle(r, col)
}

// pyDataFrame форматирует литерал pd.DataFrame со столбцами в заданном порядке.
func pyDataFrame(name string, cols []string, lits map[string]string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s = pd.DataFrame({\n", name)
	for _, c := range cols {
		fmt.Fprintf(&sb, "    \"%s\": %s,\n", c, lits[c])
	}
	sb.WriteString("})")
	return sb.String()
}

// oneHotShape — число столбцов после pd.get_dummies: уникальные значения, drop_first,
// подмножество columns= и незатронутые столбцы.
type oneHotShape struct{}

func (oneHotShape) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.feature_engineering.one-hot-shape.v1", Spec: itembank.SpecDataML, Topic: "feature_engineering",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: -0.9, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"pandas", "python"},
	}
}

func (f oneHotShape) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A50)
	nRows := itembank.IntBetween(r, 6, 8)
	cats := itembank.PickN(r, catColumns, itembank.IntBetween(r, 2, 3))
	nums := itembank.PickN(r, numColumns, itembank.IntBetween(r, 1, 2))
	dropFirst := r.IntN(2) == 0
	nEncoded := len(cats)
	if r.IntN(3) == 0 {
		nEncoded--
	}
	lits := map[string]string{}
	uniq := map[string]int{}
	cols := make([]string, 0, len(cats)+len(nums))
	for _, c := range cats {
		k := itembank.IntBetween(r, 2, min(4, len(c.values)))
		lits[c.name] = pyStrList(catSeries(r, c.values, nRows, k))
		uniq[c.name] = k
		cols = append(cols, c.name)
	}
	for _, nm := range nums {
		vals := make([]int, nRows)
		for i := range vals {
			vals[i] = itembank.IntBetween(r, 1, 60)
		}
		lits[nm] = pyIntList(vals)
		cols = append(cols, nm)
	}
	cols = itembank.Shuffle(r, cols)
	drop := 0
	if dropFirst {
		drop = 1
	}
	total := len(nums) + len(cats) - nEncoded
	encoded := make([]string, 0, nEncoded)
	parts := make([]string, 0, nEncoded)
	for _, c := range cats[:nEncoded] {
		encoded = append(encoded, c.name)
		total += uniq[c.name] - drop
		parts = append(parts, fmt.Sprintf("`%s`: уникальных значений %d, столбцов %d", c.name, uniq[c.name], uniq[c.name]-drop))
	}
	args := ""
	if nEncoded < len(cats) {
		args = ", columns=" + pyStrList(encoded)
	}
	if dropFirst {
		args += ", drop_first=True"
	}
	code := "import pandas as pd\n\n" + pyDataFrame("df", cols, lits) +
		fmt.Sprintf("\nres = pd.get_dummies(df%s)\nprint(res.shape[1])", args)
	leftover := ""
	if nEncoded < len(cats) {
		leftover = fmt.Sprintf(", а столбец `%s` не указан в `columns=` и остаётся одним столбцом", cats[nEncoded].name)
	}
	dropNote := ""
	if dropFirst {
		dropNote = " минус один, так как `drop_first=True` убирает по одному индикатору на столбец (иначе они линейно зависимы)"
	}
	stem := itembank.Pick(r, []string{
		"Сколько столбцов будет в `res`?",
		"Какое число выведет код?",
		"Чему равно `res.shape[1]` — число столбцов матрицы признаков после кодирования?",
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "python", Source: code},
		Answer: itembank.Answer{Number: float64(total)},
		Explanation: fmt.Sprintf("`get_dummies` заменяет каждый кодируемый столбец на столько бинарных столбцов, сколько у него уникальных значений%s: %s. "+
			"Остальные столбцы остаются без изменений: числовых %d%s. Итого %d. Типичные ошибки — считать число строк вместо числа уникальных значений, "+
			"забывать про `drop_first`, оставлять исходный категориальный столбец рядом с его индикаторами или кодировать все строковые столбцы, когда `columns=` ограничивает список.",
			dropNote, strings.Join(parts, "; "), len(nums), leftover, total),
	}, nil
}

// scalerTransform — результат StandardScaler/MinMaxScaler для одного значения: статистики
// только по train, делитель n, значения вне диапазона train.
type scalerTransform struct{}

func (scalerTransform) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.feature_engineering.scaler-transform.v1", Spec: itembank.SpecDataML, Topic: "feature_engineering",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.0, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"python", "sklearn"},
	}
}

// pyColumn форматирует значения как матрицу-столбец numpy: [[12.0], [18.0]].
func pyColumn(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprintf("[%d.0]", x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (f scalerTransform) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A51)
	standard := r.IntN(2) == 0
	feature := itembank.Pick(r, []string{"age", "income", "duration", "distance", "temperature", "price"})
	names := itembank.Pick(r, [][2]string{{"train", "test"}, {"X_train", "X_test"}, {"x_tr", "x_te"}})
	fromTrain := standard && r.IntN(3) == 0
	n := itembank.IntBetween(r, 5, 6)
	grid := make([]int, 56)
	for i := range grid {
		grid[i] = i + 5 // 5 … 60
	}
	var train []int
	var x, idx, lo, hi int
	var mean, std0, std1, answer float64
	for {
		train = itembank.PickN(r, grid, n)
		lo, hi = train[0], train[0]
		for _, v := range train {
			lo, hi = min(lo, v), max(hi, v)
		}
		x = itembank.IntBetween(r, 1, 70)
		if fromTrain {
			idx = r.IntN(n)
			x = train[idx]
		}
		mean = meanInts(train)
		ss := sumSqDev(train)
		std0, std1 = math.Sqrt(ss/float64(n)), math.Sqrt(ss/float64(n-1))
		if standard {
			answer = (float64(x) - mean) / std0
			if math.Abs(answer) >= 0.3 && math.Abs(answer) <= 3 {
				break
			}
			continue
		}
		if x != lo && x != hi {
			answer = float64(x-lo) / float64(hi-lo)
			break
		}
	}
	cls := "MinMaxScaler"
	if standard {
		cls = "StandardScaler"
	}
	var code strings.Builder
	fmt.Fprintf(&code, "import numpy as np\nfrom sklearn.preprocessing import %s\n\n%s = np.array(%s)  # признак \"%s\"\n", cls, names[0], pyColumn(train), feature)
	if fromTrain {
		fmt.Fprintf(&code, "scaler = %s()\nscaled = scaler.fit_transform(%s)\nprint(scaled[%d, 0])", cls, names[0], idx)
	} else {
		fmt.Fprintf(&code, "%s = np.array([[%d.0]])\nscaler = %s()\nscaler.fit(%s)\nprint(scaler.transform(%s)[0, 0])", names[1], x, cls, names[0], names[1])
	}
	var explain string
	if standard {
		where := fmt.Sprintf("Тестовое значение %d в `fit` не участвует: статистики берутся только из train.", x)
		if fromTrain {
			where = fmt.Sprintf("`fit_transform` обучает scaler на train и преобразует его же; элемент с индексом %d — это x = %d.", idx, x)
		}
		explain = fmt.Sprintf("StandardScaler запоминает по train среднее μ = %.2f и стандартное отклонение с делителем n (как `np.std`): σ = %.2f, "+
			"затем вычисляет (x − μ)/σ = (%d − %.2f)/%.2f = %.2f. %s Типичные ошибки — брать делитель n − 1, как pandas `.std()` (получилось бы %.2f), "+
			"пересчитывать статистики вместе с test или путать порядок вычитания.",
			mean, std0, x, mean, std0, answer, where, (float64(x)-mean)/std1)
	} else {
		rangeNote := "Тестовое значение лежит внутри диапазона train."
		if x < lo || x > hi {
			rangeNote = "Тестовое значение лежит за пределами диапазона train, поэтому результат выходит за отрезок [0, 1]: scaler ничего не обрезает, если не задан clip=True."
		}
		lo2, hi2 := min(lo, x), max(hi, x)
		leaky := float64(x-lo2) / float64(hi2-lo2)
		explain = fmt.Sprintf("MinMaxScaler запоминает по train min = %d и max = %d и вычисляет (x − min)/(max − min) = (%d − %d)/%d = %.2f. %s "+
			"Типичные ошибки — ограничивать результат отрезком [0, 1], пересчитывать min и max с учётом test (получилось бы %.2f) или делить на max вместо размаха.",
			lo, hi, x, lo, hi-lo, answer, rangeNote, leaky)
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Признак `%s` масштабируют. Что выведет код? Округлите до сотых.", feature),
		"Какое число выведет код? Округлите до сотых.",
		"Чему равно масштабированное значение, которое напечатает код? Ответ округлите до сотых.",
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Code:        &itembank.CodeBlock{Lang: "python", Source: code.String()},
		Answer:      itembank.Answer{Number: round2(answer), Tolerance: 0.011},
		Explanation: explain,
	}, nil
}

// targetEncoding — сглаженное целевое кодирование по train, когда у тестовых строк тоже
// известен таргет: ловушка — утечка через test.
type targetEncoding struct{}

func (targetEncoding) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.feature_engineering.target-encoding.v1", Spec: itembank.SpecDataML, Topic: "feature_engineering",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.6, B: 0.9, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"encoding"},
	}
}

type teRow struct {
	id   int
	cat  string
	y    int
	test bool
}

// teStats — число строк и сумма таргета в категории cat и по всем строкам среза
// (только train или train + test).
func teStats(rows []teRow, cat string, includeTest bool) (nCat, sumCat, nAll, sumAll int) {
	for _, row := range rows {
		if row.test && !includeTest {
			continue
		}
		nAll++
		sumAll += row.y
		if row.cat == cat {
			nCat++
			sumCat += row.y
		}
	}
	return nCat, sumCat, nAll, sumAll
}

// smoothedMean — (sumCat + k·μ)/(nCat + k), где μ — глобальное среднее таргета.
func smoothedMean(nCat, sumCat, nAll, sumAll, k int) float64 {
	mu := float64(sumAll) / float64(nAll)
	return (float64(sumCat) + float64(k)*mu) / float64(nCat+k)
}

func (f targetEncoding) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A52)
	col := itembank.Pick(r, catColumns)
	cats := itembank.PickN(r, col.values, 3)
	target := itembank.Pick(r, []string{"churn", "clicked", "converted", "returned"})
	k := itembank.Pick(r, []int{0, 1, 2, 5, 10})
	var rows []teRow
	var q teRow
	var nCat, sumCat, nAll, sumAll int
	var enc, plain, leaky float64
	for {
		nTrain, nTest := itembank.IntBetween(r, 7, 9), itembank.IntBetween(r, 2, 3)
		rows = make([]teRow, 0, nTrain+nTest)
		for i := 0; i < nTrain; i++ {
			c := itembank.Pick(r, cats)
			if i < 6 {
				c = cats[i%3] // каждая категория встречается в train хотя бы дважды
			}
			rows = append(rows, teRow{cat: c, y: r.IntN(2)})
		}
		for i := 0; i < nTest; i++ {
			rows = append(rows, teRow{cat: itembank.Pick(r, cats), y: r.IntN(2), test: true})
		}
		rows = itembank.Shuffle(r, rows)
		tests := make([]int, 0, nTest)
		for i := range rows {
			rows[i].id = i + 1
			if rows[i].test {
				tests = append(tests, i)
			}
		}
		q = rows[itembank.Pick(r, tests)]
		nCat, sumCat, nAll, sumAll = teStats(rows, q.cat, false)
		if sumAll == 0 || sumAll == nAll {
			continue
		}
		enc = smoothedMean(nCat, sumCat, nAll, sumAll, k)
		plain = float64(sumCat) / float64(nCat)
		lc, ls, la, lsa := teStats(rows, q.cat, true)
		leaky = smoothedMean(lc, ls, la, lsa, k)
		// Ловушки должны быть различимы после округления до сотых.
		if math.Abs(enc-leaky) < 0.02 || (k > 0 && math.Abs(enc-plain) < 0.02) {
			continue
		}
		break
	}
	var md strings.Builder
	fmt.Fprintf(&md, "| id | %s | %s | split |\n|---|---|---|---|\n", col.name, target)
	for _, row := range rows {
		split := "train"
		if row.test {
			split = "test"
		}
		fmt.Fprintf(&md, "| %d | %s | %d | %s |\n", row.id, row.cat, row.y, split)
	}
	rule := fmt.Sprintf("категория заменяется на сглаженное среднее целевой переменной `%s`: (n·ȳ_c + k·ȳ)/(n + k), где n — число строк этой категории в train, "+
		"ȳ_c — среднее `%s` по ним, ȳ — среднее `%s` по всему train, k = %d", target, target, target, k)
	if k == 0 {
		rule = fmt.Sprintf("категория заменяется на среднее целевой переменной `%s` по строкам этой категории в train (без сглаживания)", target)
	}
	intro := itembank.Pick(r, []string{
		fmt.Sprintf("Признак `%s` кодируют целевым кодированием (target encoding): %s. Статистики считаются только по строкам train.", col.name, rule),
		fmt.Sprintf("Для модели строят признак из столбца `%s` методом target encoding: %s; строки test в вычислении статистик не участвуют.", col.name, rule),
		fmt.Sprintf("Применяется target encoding столбца `%s`: %s. Кодировщик обучается на train и применяется к test.", col.name, rule),
	})
	mu := float64(sumAll) / float64(nAll)
	calc := fmt.Sprintf("Сглаженное значение (%d·%.2f + %d·%.2f)/(%d + %d) = %.2f.", nCat, plain, k, mu, nCat, k, enc)
	errs := fmt.Sprintf("брать несглаженное среднее %.2f или считать глобальное среднее по всей таблице", plain)
	if k == 0 {
		calc = fmt.Sprintf("Без сглаживания значение равно ȳ_c = %.2f.", plain)
		errs = "подставлять глобальное среднее ȳ или среднее категории по всей таблице"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   fmt.Sprintf("%s\n\n%s\nЧему равно закодированное значение для строки id = %d? Округлите до сотых.", intro, md.String(), q.id),
		Answer: itembank.Answer{Number: round2(enc), Tolerance: 0.011},
		Explanation: fmt.Sprintf("В train по категории «%s»: n = %d, сумма `%s` = %d, ȳ_c = %d/%d = %.2f; глобальное среднее по train ȳ = %d/%d = %.2f. %s "+
			"Тестовые строки в расчёт не входят: их `%s` — то, что модель должна предсказать; с ними получилось бы %.2f (утечка целевой переменной). "+
			"Типичные ошибки — учитывать строки test, %s.",
			q.cat, nCat, target, sumCat, sumCat, nCat, plain, sumAll, nAll, mu, calc, target, leaky, errs),
	}, nil
}

// pivotTable — результат pivot_table над маленькой таблицей: агрегат по паре ключей,
// сортировка индекса и столбцов, fill_value для отсутствующих комбинаций.
type pivotTable struct{}

func (pivotTable) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "data_ml.pandas_sql.pivot-table.v1", Spec: itembank.SpecDataML, Topic: "pandas_sql",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.5, B: 0.2, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"pandas", "python"},
	}
}

var pivotDims = []catColumn{
	{"region", []string{"east", "north", "south", "west"}},
	{"city", []string{"Kazan", "Moscow", "Perm", "Tula"}},
	{"product", []string{"laptop", "phone", "tablet", "watch"}},
	{"channel", []string{"app", "store", "web"}},
	{"quarter", []string{"Q1", "Q2", "Q3", "Q4"}},
	{"segment", []string{"new", "regular", "vip"}},
}

// aggInts применяет агрегат pandas (sum, count, max, min) к непустому списку значений.
func aggInts(agg string, xs []int) int {
	switch agg {
	case "sum":
		return sumInts(xs)
	case "count":
		return len(xs)
	}
	m := xs[0]
	for _, x := range xs {
		if agg == "max" {
			m = max(m, x)
		} else {
			m = min(m, x)
		}
	}
	return m
}

func (f pivotTable) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDA7A53)
	dims := itembank.PickN(r, pivotDims, 2)
	idxDim, colDim := dims[0], dims[1]
	valCol := itembank.Pick(r, []string{"qty", "amount", "units", "sales"})
	agg := itembank.Pick(r, []string{"sum", "count", "max", "min"})
	nIdx, nCol := itembank.IntBetween(r, 2, 3), itembank.IntBetween(r, 2, 3)
	var idxVals, colVals, idxCol, colCol []string
	var vals []int
	for {
		nRows := itembank.IntBetween(r, 7, 9)
		idxVals = itembank.PickN(r, idxDim.values, nIdx)
		colVals = itembank.PickN(r, colDim.values, nCol)
		idxCol, colCol, vals = make([]string, nRows), make([]string, nRows), make([]int, nRows)
		combos := map[string]int{}
		seenIdx, seenCol := map[string]bool{}, map[string]bool{}
		for i := range vals {
			idxCol[i], colCol[i], vals[i] = itembank.Pick(r, idxVals), itembank.Pick(r, colVals), itembank.IntBetween(r, 1, 9)
			combos[idxCol[i]+"/"+colCol[i]]++
			seenIdx[idxCol[i]], seenCol[colCol[i]] = true, true
		}
		// Нужны и повторяющиеся комбинации (чтобы агрегат имел значение), и отсутствующие
		// (чтобы сработал fill_value).
		if len(seenIdx) == nIdx && len(seenCol) == nCol && len(combos) < nRows && len(combos) < nIdx*nCol {
			break
		}
	}
	sort.Strings(idxVals)
	sort.Strings(colVals)
	result := make([][]string, 0, nIdx)
	notes := make([]string, 0, nIdx*nCol)
	for _, iv := range idxVals {
		row := []string{iv}
		for _, cv := range colVals {
			var members []int
			for i := range vals {
				if idxCol[i] == iv && colCol[i] == cv {
					members = append(members, vals[i])
				}
			}
			cell := 0
			if len(members) > 0 {
				cell = aggInts(agg, members)
				notes = append(notes, fmt.Sprintf("%s/%s: %s(%s) = %d", iv, cv, agg, intsCSV(members), cell))
			} else {
				notes = append(notes, fmt.Sprintf("%s/%s: строк нет → 0", iv, cv))
			}
			row = append(row, strconv.Itoa(cell))
		}
		result = append(result, row)
	}
	cols := []string{idxDim.name, colDim.name, valCol}
	lits := map[string]string{idxDim.name: pyStrList(idxCol), colDim.name: pyStrList(colCol), valCol: pyIntList(vals)}
	code := "import pandas as pd\n\n" + pyDataFrame("df", cols, lits) +
		fmt.Sprintf("\nres = df.pivot_table(index=\"%s\", columns=\"%s\", values=\"%s\",\n                     aggfunc=\"%s\", fill_value=0)",
			idxDim.name, colDim.name, valCol, agg)
	header := idxDim.name + ", " + strings.Join(colVals, ", ")
	colList := "`" + strings.Join(colVals, "`, `") + "`"
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Какая таблица получится в `res`? Укажите все строки в порядке выдачи, в каждой — значение `%s` и ячейки для столбцов %s в этом порядке (%s); числа записывайте целыми.",
			idxDim.name, colList, header),
		fmt.Sprintf("Выпишите результат `res` построчно в порядке выдачи: сначала значение индекса `%s`, затем ячейки для столбцов %s (%s). Числа — целые.",
			idxDim.name, colList, header),
		fmt.Sprintf("Что окажется в `res`? Перечислите строки в порядке выдачи в формате (%s), где ячейки идут в порядке столбцов %s; числа записывайте целыми.",
			header, colList),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "python", Source: code},
		Answer: itembank.Answer{Rows: result},
		Explanation: fmt.Sprintf("`pivot_table` группирует строки по паре (`%s`, `%s`), применяет %s к `%s` и разворачивает значения `%s` в столбцы; "+
			"индекс и столбцы сортируются по возрастанию, а не идут в порядке появления. Комбинации без строк получают fill_value=0 (без него там был бы NaN). "+
			"Ячейки: %s. Типичные ошибки — брать первое или последнее значение вместо агрегата, сохранять исходный порядок категорий и пропускать пустые комбинации.",
			idxDim.name, colDim.name, agg, valCol, colDim.name, strings.Join(notes, "; ")),
	}, nil
}

package backend

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(availabilityComposition{})
	itembank.Register(failingCases{})
}

// availabilityComposition — доступность цепочки сервисов с репликами.
type availabilityComposition struct{}

func (availabilityComposition) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.system_design.availability-composition.v1", Spec: itembank.SpecBackend, Topic: "system_design",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.9, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"sla", "availability"},
	}
}

func (f availabilityComposition) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xD001)
	n := itembank.IntBetween(r, 2, 3)
	names := itembank.Idents(r, "service", n)
	levels := []float64{98, 99, 99.5, 99.9, 99.95}
	avail := make([]float64, n)
	for i := range avail {
		avail[i] = itembank.Pick(r, levels)
	}
	replicated := r.IntN(n)
	replicas := itembank.IntBetween(r, 2, 3)
	if r.IntN(4) == 0 {
		replicas = 1
	}
	total := 1.0
	var parts []string
	for i := range names {
		p := avail[i] / 100
		if i == replicated && replicas > 1 {
			cluster := 1 - math.Pow(1-p, float64(replicas))
			parts = append(parts, fmt.Sprintf("%s: %d реплики → 1 − (1 − %.4g)^%d = %.8f", names[i], replicas, p, replicas, cluster))
			total *= cluster
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %.4g", names[i], p))
		total *= p
	}
	var desc []string
	for i, nm := range names {
		if i == replicated && replicas > 1 {
			desc = append(desc, fmt.Sprintf("`%s` — %g %% на экземпляр, развёрнут в %d независимых репликах за балансировщиком (достаточно одной живой)", nm, avail[i], replicas))
		} else {
			desc = append(desc, fmt.Sprintf("`%s` — %g %%", nm, avail[i]))
		}
	}
	askMinutes := r.IntN(2) == 0
	percent := total * 100
	downtime := (1 - total) * 30 * 24 * 60
	var question string
	var answer, tol float64
	if askMinutes {
		question = "Сколько минут простоя в месяц (30 дней) допускает такая доступность? Ответ округлите до целых минут."
		answer, tol = math.Round(downtime), 1.0
	} else {
		question = "Какова результирующая доступность запроса в процентах? Ответ укажите с точностью до сотых."
		answer, tol = math.Round(percent*100)/100, 0.01
	}
	stems := []string{
		fmt.Sprintf("Запрос обрабатывается цепочкой сервисов, и для успеха должны работать все звенья: %s. Отказы независимы, балансировщик и сеть считаем идеально надёжными. %s", strings.Join(desc, "; "), question),
		fmt.Sprintf("Доступность сервисов в синхронной цепочке вызовов: %s. Запрос успешен, только если ответили все звенья; отказы независимы, остальная инфраструктура безотказна. %s", strings.Join(desc, "; "), question),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Answer: itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: fmt.Sprintf("Последовательные зависимости перемножаются, параллельные реплики повышают доступность: %s. Произведение = %.6f, то есть %.2f %%, "+
			"или %.0f минут простоя за 30 дней (43 200 минут). Типичная ошибка — брать минимум по цепочке или складывать проценты недоступности реплик.",
			strings.Join(parts, "; "), total, percent, downtime),
	}, nil
}

// failingCases — какие кейсы табличного теста упадут на реализации с ошибкой.
type failingCases struct{}

func (failingCases) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.testing.failing-cases.v1", Spec: itembank.SpecBackend, Topic: "testing",
		Grade: itembank.GradeMiddle, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.3, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"go", "table-tests"},
	}
}

// testCase — строка табличного теста: аргументы для кода, подпись для варианта,
// ожидаемое значение по спецификации и фактическое значение реализации с ошибкой.
type testCase struct {
	row   string
	label string
	want  string
	got   string
}

func pagesCorrect(total, size int) int { return (total + size - 1) / size }
func pagesBuggy(total, size int) int   { return total / size }

func inRangeCorrect(x, lo, hi int) bool { return x >= lo && x <= hi }
func inRangeBuggy(x, lo, hi int) bool   { return x >= lo && x < hi }

func roundCorrect(n int) int { return (n + 50) / 100 * 100 }
func roundBuggy(n int) int   { return n / 100 * 100 }

func truncateCorrect(s string, n int) string {
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

func truncateBuggy(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func (f failingCases) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xD002)
	between := func(lo, hi int) int { return itembank.IntBetween(r, lo, hi) }
	words := []string{"configuration", "authentication", "serialization", "notifications", "infrastructure", "observability"}
	specs := []struct {
		testName, impl, fields, call, verb, bug string
		gen                                     func(fail bool) testCase
	}{
		{
			testName: "Pages",
			impl:     "// pages возвращает число страниц для total элементов при size элементов на странице.\nfunc pages(total, size int) int {\n\treturn total / size\n}",
			fields:   "total, size int\n\t\twant        int",
			call:     "pages(c.total, c.size)", verb: "%d",
			bug: "целочисленное деление отбрасывает неполную последнюю страницу (нужно округление вверх)",
			gen: func(fail bool) testCase {
				size := itembank.Pick(r, []int{5, 10, 20, 25})
				total := size * between(0, 5)
				if fail {
					total += between(1, size-1)
				}
				return testCase{row: fmt.Sprintf("%d, %d, %d", total, size, pagesCorrect(total, size)),
					label: fmt.Sprintf("pages(%d, %d)", total, size),
					want:  strconv.Itoa(pagesCorrect(total, size)), got: strconv.Itoa(pagesBuggy(total, size))}
			},
		},
		{
			testName: "InRange",
			impl:     "// inRange сообщает, лежит ли x в отрезке [lo, hi] включительно с обеих сторон.\nfunc inRange(x, lo, hi int) bool {\n\treturn x >= lo && x < hi\n}",
			fields:   "x, lo, hi int\n\t\twant      bool",
			call:     "inRange(c.x, c.lo, c.hi)", verb: "%v",
			bug: "правая граница сравнивается строго (<), хотя отрезок включает hi",
			gen: func(fail bool) testCase {
				lo := between(0, 20)
				hi := lo + between(3, 30)
				var x int
				if fail {
					x = hi
				} else {
					switch between(0, 3) {
					case 0:
						x = lo
					case 1:
						x = lo - between(1, 5)
					case 2:
						x = hi + between(1, 5)
					default:
						x = lo + between(1, hi-lo-1)
					}
				}
				return testCase{row: fmt.Sprintf("%d, %d, %d, %v", x, lo, hi, inRangeCorrect(x, lo, hi)),
					label: fmt.Sprintf("inRange(%d, %d, %d)", x, lo, hi),
					want:  strconv.FormatBool(inRangeCorrect(x, lo, hi)), got: strconv.FormatBool(inRangeBuggy(x, lo, hi))}
			},
		},
		{
			testName: "RoundTo100",
			impl:     "// roundTo100 округляет n до ближайшей сотни; ровно половина (…50) округляется вверх.\nfunc roundTo100(n int) int {\n\treturn n / 100 * 100\n}",
			fields:   "n    int\n\t\twant int",
			call:     "roundTo100(c.n)", verb: "%d",
			bug: "деление с отбрасыванием всегда округляет вниз, поэтому остаток 50 и больше теряется",
			gen: func(fail bool) testCase {
				n := between(1, 9) * 100
				if fail {
					n += between(50, 99)
				} else {
					n += between(0, 49)
				}
				return testCase{row: fmt.Sprintf("%d, %d", n, roundCorrect(n)),
					label: fmt.Sprintf("roundTo100(%d)", n),
					want:  strconv.Itoa(roundCorrect(n)), got: strconv.Itoa(roundBuggy(n))}
			},
		},
		{
			testName: "Truncate",
			impl:     "// truncate возвращает s, если она не длиннее n, иначе первые n-3 символа и \"...\",\n// чтобы результат умещался в n символов.\nfunc truncate(s string, n int) string {\n\tif len(s) > n {\n\t\treturn s[:n] + \"...\"\n\t}\n\treturn s\n}",
			fields:   "s    string\n\t\tn    int\n\t\twant string",
			call:     "truncate(c.s, c.n)", verb: "%q",
			bug: "при обрезке берутся n символов и добавляется многоточие, результат получается длиннее n",
			gen: func(fail bool) testCase {
				n := between(5, 9)
				length := between(2, n)
				if fail {
					length = n + between(1, 4)
				}
				s := itembank.Pick(r, words)[:length]
				return testCase{row: fmt.Sprintf("%q, %d, %q", s, n, truncateCorrect(s, n)),
					label: fmt.Sprintf("truncate(%q, %d)", s, n),
					want:  truncateCorrect(s, n), got: truncateBuggy(s, n)}
			},
		},
	}
	spec := itembank.Pick(r, specs)
	failCount := between(2, 3)
	const total = 5
	var cases []testCase
	seenLabel := map[string]bool{}
	for len(cases) < total {
		c := spec.gen(len(cases) < failCount)
		if seenLabel[c.label] {
			continue
		}
		seenLabel[c.label] = true
		cases = append(cases, c)
	}
	cases = itembank.Shuffle(r, cases)
	names := []string{"A", "B", "C", "D", "E"}
	var rows, correct, distractors, failed []string
	for i, c := range cases {
		rows = append(rows, fmt.Sprintf("{%q, %s},", names[i], c.row))
		opt := names[i] + " — " + c.label
		if c.got != c.want {
			correct = append(correct, opt)
			failed = append(failed, fmt.Sprintf("%s: получено %s, ожидалось %s", names[i], c.got, c.want))
		} else {
			distractors = append(distractors, opt)
		}
	}
	if len(correct) != failCount {
		return itembank.Instance{}, fmt.Errorf("seed %d: expected %d failing cases, got %d", seed, failCount, len(correct))
	}
	code := fmt.Sprintf(`%s

func Test%s(t *testing.T) {
	cases := []struct {
		name string
		%s
	}{
		%s
	}
	for _, c := range cases {
		if got := %s; got != c.want {
			t.Errorf("%%s: got %s, want %s", c.name, got, c.want)
		}
	}
}`, spec.impl, spec.testName, spec.fields, strings.Join(rows, "\n\t\t"), spec.call, spec.verb, spec.verb)
	options, keys := itembank.ShuffledMulti(r, correct, distractors...)
	stems := []string{
		"Ожидаемые значения в таблице соответствуют спецификации из комментария, а реализация содержит ошибку. Какие кейсы теста упадут? Отметьте все.",
		"Комментарий к функции описывает требуемое поведение, колонка want заполнена по нему верно. Отметьте все кейсы, на которых `go test` сообщит об ошибке.",
		"Табличный тест ниже проверяет функцию с дефектом; ожидания в таблице корректны. Какие строки таблицы не пройдут?",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "go", Source: code},
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: fmt.Sprintf("Дефект: %s. Падают кейсы — %s. Остальные входы не задевают граничное условие, и ошибочная реализация совпадает с ожидаемой. "+
			"Типичная ошибка — отмечать кейсы по «подозрительности» входа, а не прогоняя реализацию вручную.", spec.bug, strings.Join(failed, "; ")),
	}, nil
}

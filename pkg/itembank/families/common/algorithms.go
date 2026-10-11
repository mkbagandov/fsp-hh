package common

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(recursionComplexity{})
	itembank.Register(amortizedResize{})
	itembank.Register(dequeOps{})
	itembank.Register(hashCollisions{})
	itembank.Register(bstTraversal{})
	itembank.Register(bfsDistance{})
}

// joinInts соединяет числа через sep.
func joinInts(xs []int, sep string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, sep)
}

// distinctInts возвращает n различных целых из [lo, hi] (требуется hi − lo + 1 ≥ n).
func distinctInts(r *rand.Rand, n, lo, hi int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, n)
	for len(out) < n {
		v := itembank.IntBetween(r, lo, hi)
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// pluralRu подбирает форму существительного к числу: 1 коммит, 2 коммита, 5 коммитов.
func pluralRu(n int, one, few, many string) string {
	switch {
	case n%100 >= 11 && n%100 <= 19:
		return many
	case n%10 == 1:
		return one
	case n%10 >= 2 && n%10 <= 4:
		return few
	default:
		return many
	}
}

// recursionComplexity — сложность рекурсивного алгоритма или сортировки по словесному
// описанию: кандидат должен восстановить рекуррентное соотношение и решить его.
type recursionComplexity struct{}

func (recursionComplexity) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.complexity.recursion-complexity.v1", Spec: itembank.SpecCommon, Topic: "complexity",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.2, C: 0.25}, TimeLimit: 90 * time.Second, Tags: []string{"big-o", "recursion"},
	}
}

type recursionCase struct {
	label   string
	texts   []string // шаблоны: %[1]s — имя функции, %[2]s — имя входных данных
	explain string
}

var complexityLabels = []string{"O(log n)", "O(n)", "O(n log n)", "O(n²)", "O(n³)", "O(2ⁿ)"}

var recursionCases = []recursionCase{
	{
		label: "O(log n)",
		texts: []string{
			"Функция `%[1]s` ищет значение в отсортированном массиве `%[2]s`: сравнивает его с серединой и рекурсивно продолжает поиск только в одной из половин.",
			"`%[1]s` на каждом шаге отбрасывает половину диапазона `%[2]s` и делает один рекурсивный вызов для оставшейся половины; остальная работа — константная.",
		},
		explain: "Рекуррентное соотношение T(n) = T(n/2) + O(1): на каждом уровне константная работа, а уровней log₂ n.",
	},
	{
		label: "O(n log n)",
		texts: []string{
			"Функция `%[1]s` делит `%[2]s` пополам, рекурсивно обрабатывает обе половины и объединяет результаты за линейное время.",
			"`%[1]s` сортирует `%[2]s`, разделяя его на две равные части, сортируя каждую рекурсивно и сливая отсортированные части за O(n).",
		},
		explain: "T(n) = 2T(n/2) + O(n): на каждом из log₂ n уровней рекурсии суммарная работа линейна — это сортировка слиянием.",
	},
	{
		label: "O(n)",
		texts: []string{
			"Функция `%[1]s` делит `%[2]s` пополам, рекурсивно обрабатывает обе половины и объединяет их результаты за константное время.",
			"`%[1]s` делает два рекурсивных вызова для половин `%[2]s` и возвращает сумму их результатов; кроме этого выполняется O(1) операций.",
		},
		explain: "T(n) = 2T(n/2) + O(1): число вызовов удваивается на каждом уровне, всего около 2n вызовов с константной работой — линейная сложность, а не n log n.",
	},
	{
		label: "O(n)",
		texts: []string{
			"Функция `%[1]s` обрабатывает первый элемент `%[2]s` за константное время и рекурсивно вызывает себя для остатка, который короче на один элемент.",
			"`%[1]s` реализована рекурсивно: один вызов для `%[2]s` без последнего элемента плюс константная работа с этим элементом.",
		},
		explain: "T(n) = T(n − 1) + O(1): глубина рекурсии n, на каждом уровне константная работа — Θ(n).",
	},
	{
		label: "O(n²)",
		texts: []string{
			"Функция `%[1]s` за линейный проход находит минимум `%[2]s`, ставит его в начало и рекурсивно сортирует оставшиеся n − 1 элементов.",
			"`%[1]s` на каждом уровне рекурсии просматривает весь текущий фрагмент `%[2]s`, после чего вызывает себя для фрагмента, укороченного на один элемент.",
		},
		explain: "T(n) = T(n − 1) + O(n): n + (n − 1) + … + 1 = n(n + 1)/2 — квадратичная сложность (сортировка выбором).",
	},
	{
		label: "O(2ⁿ)",
		texts: []string{
			"Функция `%[1]s` для входа размера n делает два рекурсивных вызова для размера n − 1 над `%[2]s` и объединяет их результаты за константное время.",
			"`%[1]s(n)` возвращает `%[1]s(n − 1) + %[1]s(n − 1)` без мемоизации; в базовом случае читает один элемент `%[2]s`.",
		},
		explain: "T(n) = 2T(n − 1) + O(1): дерево вызовов удваивается на каждом из n уровней, всего около 2ⁿ вызовов (наивное вычисление чисел Фибоначчи).",
	},
	{
		label: "O(n²)",
		texts: []string{
			"Функция `%[1]s` разбивает `%[2]s` на две половины, делает четыре рекурсивных вызова для половин и объединяет результаты за линейное время.",
			"`%[1]s` сводит задачу размера n над `%[2]s` к четырём подзадачам размера n/2 плюс O(n) дополнительной работы.",
		},
		explain: "T(n) = 4T(n/2) + O(n): по основной теореме log₂ 4 = 2 > 1, доминируют листья дерева рекурсии — Θ(n²).",
	},
	{
		label: "O(n³)",
		texts: []string{
			"Функция `%[1]s` сводит задачу над `%[2]s` к восьми подзадачам половинного размера и объединяет их результаты за квадратичное время.",
			"`%[1]s` делает 8 рекурсивных вызовов для входа вдвое меньшего размера, чем `%[2]s`, и тратит O(n²) на объединение.",
		},
		explain: "T(n) = 8T(n/2) + O(n²): log₂ 8 = 3 > 2, итог Θ(n³) — блочное умножение матриц без оптимизаций.",
	},
	{
		label: "O(n)",
		texts: []string{
			"Функция `%[1]s` за линейное время разбивает `%[2]s` относительно опорного элемента и рекурсивно обрабатывает только одну часть, размер которой каждый раз уменьшается вдвое.",
			"`%[1]s` на каждом уровне просматривает весь текущий фрагмент `%[2]s`, а затем делает один рекурсивный вызов для его половины.",
		},
		explain: "T(n) = T(n/2) + O(n): n + n/2 + n/4 + … < 2n — линейная сложность (поиск k-го по порядку элемента).",
	},
	{
		label: "O(n²)",
		texts: []string{
			"Функция `%[1]s` разбивает `%[2]s` относительно опорного элемента за линейное время и рекурсивно сортирует обе части; в худшем случае опорным каждый раз оказывается минимальный элемент. Оцените сложность худшего случая.",
			"`%[1]s` — быстрая сортировка `%[2]s`, где опорным всегда берётся первый элемент, а массив уже отсортирован. Оцените сложность в этом случае.",
		},
		explain: "При вырожденном разбиении одна часть пуста, другая содержит n − 1 элементов: T(n) = T(n − 1) + O(n) = Θ(n²) — худший случай быстрой сортировки.",
	},
	{
		label: "O(n²)",
		texts: []string{
			"Функция `%[1]s` сортирует `%[2]s`: за один проход сравнивает и при необходимости меняет местами соседние элементы, проходов выполняется n − 1.",
			"`%[1]s` многократно проходит по `%[2]s`, поднимая максимум оставшейся части в конец, пока массив не станет отсортированным (n − 1 проход).",
		},
		explain: "n − 1 проходов по массиву из n элементов: около n²/2 сравнений — Θ(n²), пузырьковая сортировка.",
	},
	{
		label: "O(n log n)",
		texts: []string{
			"Функция `%[1]s` строит из `%[2]s` двоичную кучу, а затем n раз извлекает максимум, записывая его в конец.",
			"`%[1]s` сортирует `%[2]s`, помещая все элементы в двоичную кучу и извлекая их по одному.",
		},
		explain: "Построение кучи — O(n), каждое из n извлечений — O(log n): итого Θ(n log n), пирамидальная сортировка.",
	},
	{
		label: "O(n log n)",
		texts: []string{
			"Функция `%[1]s` делит `%[2]s` на части из 1/3 и 2/3 элементов, рекурсивно обрабатывает обе и объединяет за линейное время.",
			"`%[1]s` разбивает `%[2]s` в пропорции 1:2, рекурсивно вызывает себя для обеих частей и тратит O(n) на слияние.",
		},
		explain: "Глубина рекурсии log₃⁄₂ n, на каждом уровне суммарно линейная работа — всё ещё Θ(n log n): неравное деление меняет константу, а не класс.",
	},
	{
		label: "O(n²)",
		texts: []string{
			"Функция `%[1]s` сортирует `%[2]s` вставками: каждый очередной элемент сдвигается влево, пока не встанет на своё место; массив изначально отсортирован по убыванию.",
			"`%[1]s` реализует сортировку вставками для `%[2]s`; оцените сложность для входа, упорядоченного в обратном порядке.",
		},
		explain: "В худшем случае i-й элемент сдвигается на i позиций: 1 + 2 + … + (n − 1) = n(n − 1)/2 — Θ(n²).",
	},
}

func (f recursionComplexity) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE10)
	c := itembank.Pick(r, recursionCases)
	fn := itembank.Ident(r, "func")
	data := itembank.Pick(r, []string{"arr", "items", "data", "xs", "values", "nums"})
	text := fmt.Sprintf(itembank.Pick(r, c.texts), fn, data)
	var distractors []string
	for _, l := range complexityLabels {
		if l != c.label {
			distractors = append(distractors, l)
		}
	}
	options, key := itembank.ShuffledOptions(r, c.label, itembank.PickN(r, distractors, 3)...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    text + " Какова асимптотическая временная сложность относительно n — размера входа?",
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: c.explain + " Типичная ошибка — судить по числу рекурсивных вызовов в коде, не учитывая, как быстро уменьшается " +
			"размер задачи и сколько работы делается вне рекурсии.",
	}, nil
}

// amortizedResize — амортизированный анализ динамического массива: число реаллокаций,
// суммарное число копирований и итоговая ёмкость вычисляются моделированием.
type amortizedResize struct{}

func (amortizedResize) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.complexity.amortized-resize.v1", Spec: itembank.SpecCommon, Topic: "complexity",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.9, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"big-o", "amortized"},
	}
}

func (f amortizedResize) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE11)
	double := r.IntN(3) != 0
	var initial int
	var growText string
	if double {
		initial = itembank.Pick(r, []int{1, 2, 4, 8})
		growText = "вдвое больше прежней"
	} else {
		initial = itembank.Pick(r, []int{2, 4, 8})
		growText = "⌊1,5 · прежняя⌋ (с округлением вниз)"
	}
	n := itembank.IntBetween(r, 9, 64)
	name := itembank.Pick(r, []string{"items", "buf", "list", "vec", "data"})

	capacity, size, copies := initial, 0, 0
	var caps []int
	for i := 0; i < n; i++ {
		if size == capacity {
			copies += size
			if double {
				capacity *= 2
			} else {
				capacity = capacity * 3 / 2
			}
			caps = append(caps, capacity)
		}
		size++
	}
	resizes := len(caps)

	var question string
	var answer int
	switch r.IntN(3) {
	case 0:
		question = "Сколько раз за это время произойдёт перевыделение буфера (реаллокация)?"
		answer = resizes
	case 1:
		question = "Сколько всего элементов будет скопировано из старого буфера в новый за все реаллокации в сумме?"
		answer = copies
	default:
		question = "Какой будет ёмкость буфера после всех добавлений?"
		answer = capacity
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Динамический массив `%s` создан с ёмкостью %d и без элементов. Когда при добавлении свободного места нет, "+
			"выделяется новый буфер ёмкостью %s, все текущие элементы копируются в него, и только затем добавляется новый. "+
			"В массив по одному добавляют %d элементов. %s", name, initial, growText, n, question),
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Ёмкость после каждой реаллокации: %d → %s; всего %d реаллокаций, скопировано %d элементов, итоговая ёмкость %d. "+
			"Копирований меньше, чем c·n (геометрическая прогрессия), поэтому амортизированная стоимость добавления — O(1), хотя отдельная "+
			"операция стоит O(n). Типичная ошибка — считать копирование при каждом добавлении или включать в копирование сам добавляемый элемент.",
			initial, joinInts(caps, " → "), resizes, copies, capacity),
	}, nil
}

// dequeOps — состояние очереди или дека после последовательности операций.
type dequeOps struct{}

func (dequeOps) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.data_structures.deque-ops.v1", Spec: itembank.SpecCommon, Topic: "data_structures",
		Grade: itembank.GradeJunior, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.0, B: -0.8, C: 0}, TimeLimit: 60 * time.Second,
	}
}

func (f dequeOps) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE12)
	isQueue := r.IntN(2) == 0
	n := itembank.IntBetween(r, 6, 9)
	vals := distinctInts(r, n, 1, 99)
	var dq []int
	var ops []string
	next := 0
	for i := 0; i < n; i++ {
		// Удаление допустимо только из непустой структуры; последняя операция — добавление.
		canPop := len(dq) > 0 && i < n-1 && r.IntN(3) == 0
		switch {
		case isQueue && canPop:
			dq = dq[1:]
			ops = append(ops, "dequeue()")
		case isQueue:
			dq = append(dq, vals[next])
			ops = append(ops, fmt.Sprintf("enqueue(%d)", vals[next]))
			next++
		case canPop && r.IntN(2) == 0:
			dq = dq[1:]
			ops = append(ops, "pop_front()")
		case canPop:
			dq = dq[:len(dq)-1]
			ops = append(ops, "pop_back()")
		case r.IntN(2) == 0:
			dq = slices.Insert(dq, 0, vals[next])
			ops = append(ops, fmt.Sprintf("push_front(%d)", vals[next]))
			next++
		default:
			dq = append(dq, vals[next])
			ops = append(ops, fmt.Sprintf("push_back(%d)", vals[next]))
			next++
		}
	}
	dequeIntro := "Над пустым двусторонним деком последовательно выполнили операции (`push_front` добавляет в начало, `push_back` — в конец, " +
		"`pop_front` и `pop_back` удаляют первый и последний элемент): "
	var intro, question, where string
	answer := dq[0]
	switch {
	case isQueue:
		intro = "Над пустой очередью (FIFO) последовательно выполнили операции: "
		question = "Какое число вернёт следующий вызов `dequeue()`?"
		where = "в начале очереди"
	case r.IntN(2) == 0:
		intro = dequeIntro
		question = "Какое число находится в начале дека (его вернул бы `pop_front()`)?"
		where = "в начале дека"
	default:
		intro = dequeIntro
		question = "Какое число находится в конце дека (его вернул бы `pop_back()`)?"
		where = "в конце дека"
		answer = dq[len(dq)-1]
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   intro + "`" + strings.Join(ops, "`, `") + "`. " + question + " Ответ — число.",
		Answer: itembank.Answer{Text: strconv.Itoa(answer)},
		Explanation: fmt.Sprintf("Содержимое после всех операций (от начала к концу): [%s]; %s находится %d. Очередь удаляет элементы с противоположной "+
			"добавлению стороны (FIFO), дек — с любой, поэтому важно следить, в какой конец идёт каждая операция. Типичная ошибка — обрабатывать очередь "+
			"как стек или путать, в какую сторону добавляет push_front.", joinInts(dq, ", "), where, answer),
	}, nil
}

// hashCollisions — распределение ключей по бакетам при h(k) = k mod m: коллизии, длина
// цепочки, пустые бакеты, коэффициент заполнения, слот при линейном пробировании.
type hashCollisions struct{}

func (hashCollisions) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.data_structures.hash-collisions.v1", Spec: itembank.SpecCommon, Topic: "data_structures",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.1, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"hash-table"},
	}
}

func (f hashCollisions) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE13)
	probing := r.IntN(3) == 0
	var m, n int
	if probing {
		m = itembank.Pick(r, []int{7, 8, 10, 11, 13})
		n = itembank.IntBetween(r, 5, min(9, m-2))
	} else {
		m = itembank.Pick(r, []int{5, 7, 8, 10, 11, 13})
		n = itembank.IntBetween(r, 5, 9)
	}
	keys := distinctInts(r, n, 1, 99)
	keysText := joinInts(keys, ", ")

	if probing {
		return f.probingInstance(r, seed, m, keys, keysText), nil
	}

	buckets := make([][]int, m)
	collisions := 0
	for _, k := range keys {
		b := k % m
		if len(buckets[b]) > 0 {
			collisions++
		}
		buckets[b] = append(buckets[b], k)
	}
	longest, empty := 0, 0
	var filled []string
	for i, b := range buckets {
		if len(b) == 0 {
			empty++
			continue
		}
		longest = max(longest, len(b))
		filled = append(filled, fmt.Sprintf("%d → [%s]", i, joinInts(b, ", ")))
	}
	load := float64(n) / float64(m)
	var question string
	answer := itembank.Answer{}
	switch r.IntN(4) {
	case 0:
		question = "Сколько коллизий произойдёт при вставке (коллизия — вставка в уже непустой бакет)?"
		answer.Number = float64(collisions)
	case 1:
		question = "Какова длина самой длинной цепочки после всех вставок?"
		answer.Number = float64(longest)
	case 2:
		question = "Сколько бакетов останутся пустыми?"
		answer.Number = float64(empty)
	default:
		question = "Каким будет коэффициент заполнения (load factor = число элементов / число бакетов)? Ответ — десятичная дробь, достаточно двух знаков после запятой."
		answer.Number = load
		answer.Tolerance = 0.01
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Хеш-таблица из %d бакетов с индексами 0…%d, хеш-функция `h(k) = k mod %d`, коллизии разрешаются методом цепочек "+
			"(список в каждом бакете). В пустую таблицу по очереди вставляют ключи %s. %s", m, m-1, m, keysText, question),
		Answer: answer,
		Explanation: fmt.Sprintf("Непустые бакеты: %s. Коллизий — %d (каждая вставка в непустой бакет), самая длинная цепочка — %d, пустых бакетов — %d, "+
			"load factor = %d/%d = %.2f. Типичная ошибка — считать коллизией каждый элемент цепочки, включая первый, или вычислять load factor "+
			"как долю занятых бакетов.", strings.Join(filled, "; "), collisions, longest, empty, n, m, load),
	}, nil
}

// probingInstance — вариант с открытой адресацией: индекс слота или число проверок для последнего ключа.
func (f hashCollisions) probingInstance(r *rand.Rand, seed uint64, m int, keys []int, keysText string) itembank.Instance {
	slots := make([]int, m)
	for i := range slots {
		slots[i] = -1
	}
	lastIdx, lastProbes := 0, 0
	var lastPath []int
	for _, k := range keys {
		idx := k % m
		probes := 1
		path := []int{idx}
		for slots[idx] >= 0 {
			idx = (idx + 1) % m
			probes++
			path = append(path, idx)
		}
		slots[idx] = k
		lastIdx, lastProbes, lastPath = idx, probes, path
	}
	last := keys[len(keys)-1]
	var question string
	var answer int
	if r.IntN(2) == 0 {
		question = fmt.Sprintf("В слот с каким индексом будет записан последний ключ %d?", last)
		answer = lastIdx
	} else {
		question = fmt.Sprintf("Сколько слотов будет проверено при вставке последнего ключа %d (включая слот, в который он записан)?", last)
		answer = lastProbes
	}
	var occupied []string
	for i, v := range slots {
		if v >= 0 {
			occupied = append(occupied, fmt.Sprintf("%d → %d", i, v))
		}
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Хеш-таблица из %d слотов с индексами 0…%d, хеш-функция `h(k) = k mod %d`, коллизии разрешаются открытой адресацией "+
			"с линейным пробированием: если слот занят, проверяется следующий (индекс + 1 по модулю %d). В пустую таблицу по очереди вставляют ключи %s. %s",
			m, m-1, m, m, keysText, question),
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Занятые слоты после вставок: %s. Для ключа %d h = %d, проверенные слоты: %s — записан в слот %d за %d проверок. "+
			"Типичная ошибка — не учитывать переход через конец таблицы по модулю или считать только занятые слоты без итогового.",
			strings.Join(occupied, ", "), last, last%m, joinInts(lastPath, " → "), lastIdx, lastProbes),
	}
}

// bstTraversal — обход двоичного дерева поиска, построенного вставками в заданном порядке.
type bstTraversal struct{}

func (bstTraversal) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.data_structures.bst-traversal.v1", Spec: itembank.SpecCommon, Topic: "data_structures",
		Grade: itembank.GradeMiddle, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.2, B: 0.3, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"tree"},
	}
}

type bstNode struct {
	key         int
	left, right *bstNode
}

func bstInsert(n *bstNode, key int) *bstNode {
	if n == nil {
		return &bstNode{key: key}
	}
	if key < n.key {
		n.left = bstInsert(n.left, key)
	} else {
		n.right = bstInsert(n.right, key)
	}
	return n
}

// bstWalk — рекурсивный обход: 0 — pre-order, 1 — post-order, иначе in-order.
func bstWalk(n *bstNode, mode int, out *[]int) {
	if n == nil {
		return
	}
	switch mode {
	case 0:
		*out = append(*out, n.key)
		bstWalk(n.left, mode, out)
		bstWalk(n.right, mode, out)
	case 1:
		bstWalk(n.left, mode, out)
		bstWalk(n.right, mode, out)
		*out = append(*out, n.key)
	default:
		bstWalk(n.left, mode, out)
		*out = append(*out, n.key)
		bstWalk(n.right, mode, out)
	}
}

func bstLevelOrder(root *bstNode) []int {
	var out []int
	queue := []*bstNode{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		out = append(out, n.key)
		if n.left != nil {
			queue = append(queue, n.left)
		}
		if n.right != nil {
			queue = append(queue, n.right)
		}
	}
	return out
}

func bstString(n *bstNode) string {
	if n.left == nil && n.right == nil {
		return strconv.Itoa(n.key)
	}
	var parts []string
	if n.left != nil {
		parts = append(parts, "L: "+bstString(n.left))
	}
	if n.right != nil {
		parts = append(parts, "R: "+bstString(n.right))
	}
	return fmt.Sprintf("%d (%s)", n.key, strings.Join(parts, ", "))
}

func (f bstTraversal) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE14)
	n := itembank.IntBetween(r, 6, 7)
	keys := distinctInts(r, n, 1, 99)
	var root *bstNode
	for _, k := range keys {
		root = bstInsert(root, k)
	}
	var result []int
	var modeText, modeName string
	switch r.IntN(7) {
	case 0, 1:
		bstWalk(root, 0, &result)
		modeText = "прямого обхода (pre-order: узел → левое поддерево → правое поддерево)"
		modeName = "Прямой обход"
	case 2, 3:
		bstWalk(root, 1, &result)
		modeText = "обратного обхода (post-order: левое поддерево → правое поддерево → узел)"
		modeName = "Обратный обход"
	case 4, 5:
		result = bstLevelOrder(root)
		modeText = "обхода в ширину (по уровням сверху вниз, на каждом уровне слева направо)"
		modeName = "Обход в ширину"
	default:
		bstWalk(root, 2, &result)
		modeText = "симметричного обхода (in-order: левое поддерево → узел → правое поддерево)"
		modeName = "Симметричный обход"
	}
	answer := joinInts(result, " ")
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("В пустое двоичное дерево поиска (без балансировки: ключ меньше узла — в левое поддерево, больше — в правое) последовательно "+
			"вставляют ключи: %s. Запишите ключи в порядке %s через пробел.", joinInts(keys, ", "), modeText),
		Answer: itembank.Answer{Text: answer},
		Explanation: fmt.Sprintf("Корень — %d (первый вставленный ключ); структура дерева: %s. %s даёт: %s. Симметричный обход BST всегда выдаёт ключи "+
			"по возрастанию, остальные обходы зависят от формы дерева. Типичная ошибка — вставлять ключи как в отсортированный список или путать момент "+
			"посещения узла относительно поддеревьев.", keys[0], bstString(root), modeName, answer),
	}, nil
}

// bfsDistance — расстояние в рёбрах между вершинами небольшого графа (обход в ширину).
type bfsDistance struct{}

func (bfsDistance) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.data_structures.bfs-distance.v1", Spec: itembank.SpecCommon, Topic: "data_structures",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.1, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"graph"},
	}
}

func bfsFrom(adj [][]int, s int) []int {
	dist := make([]int, len(adj))
	for i := range dist {
		dist[i] = -1
	}
	dist[s] = 0
	queue := []int{s}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, w := range adj[v] {
			if dist[w] < 0 {
				dist[w] = dist[v] + 1
				queue = append(queue, w)
			}
		}
	}
	return dist
}

func (f bfsDistance) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE15)
	n := itembank.IntBetween(r, 6, 8)
	var names []string
	if r.IntN(2) == 0 {
		names = []string{"A", "B", "C", "D", "E", "F", "G", "H"}[:n]
	} else {
		names = itembank.Idents(r, "city", n)
	}
	type edge struct{ a, b int }
	var edges []edge
	has := map[[2]int]bool{}
	add := func(a, b int) bool {
		if a == b {
			return false
		}
		if a > b {
			a, b = b, a
		}
		if has[[2]int{a, b}] {
			return false
		}
		has[[2]int{a, b}] = true
		edges = append(edges, edge{a, b})
		return true
	}
	// Остовное дерево гарантирует связность, затем 1–3 дополнительных ребра.
	for i := 1; i < n; i++ {
		add(i, r.IntN(i))
	}
	for extra := itembank.IntBetween(r, 1, 3); extra > 0; extra-- {
		for tries := 0; tries < 10; tries++ {
			if add(r.IntN(n), r.IntN(n)) {
				break
			}
		}
	}
	adj := make([][]int, n)
	for _, e := range edges {
		adj[e.a] = append(adj[e.a], e.b)
		adj[e.b] = append(adj[e.b], e.a)
	}
	shown := itembank.Shuffle(r, edges)
	edgeTexts := make([]string, len(shown))
	for i, e := range shown {
		edgeTexts[i] = names[e.a] + "–" + names[e.b]
	}
	s := r.IntN(n)
	dist := bfsFrom(adj, s)
	maxDist := 0
	for _, d := range dist {
		maxDist = max(maxDist, d)
	}
	layers := make([][]string, maxDist+1)
	for v, d := range dist {
		layers[d] = append(layers[d], names[v])
	}
	layerTexts := make([]string, len(layers))
	for d, l := range layers {
		layerTexts[d] = fmt.Sprintf("слой %d — %s", d, strings.Join(l, ", "))
	}

	var question string
	var answer int
	if r.IntN(3) < 2 {
		var far []int
		for v, d := range dist {
			if d >= 2 {
				far = append(far, v)
			}
		}
		t := (s + 1) % n
		if len(far) > 0 {
			t = itembank.Pick(r, far)
		}
		question = fmt.Sprintf("Какова длина кратчайшего пути (в рёбрах) от %s до %s?", names[s], names[t])
		answer = dist[t]
	} else {
		d := itembank.IntBetween(r, 1, maxDist)
		question = fmt.Sprintf("Сколько вершин находятся на расстоянии ровно %d %s от %s?", d, pluralRu(d, "ребро", "ребра", "рёбер"), names[s])
		answer = len(layers[d])
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Неориентированный граф с вершинами %s задан списком рёбер: %s. %s",
			strings.Join(names, ", "), strings.Join(edgeTexts, ", "), question),
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Обход в ширину от %s раскладывает вершины по слоям (расстояние = номер слоя): %s. Типичная ошибка — считать путь "+
			"по порядку перечисления рёбер, брать число вершин на пути вместо числа рёбер или пропускать более короткий путь через дополнительное ребро.",
			names[s], strings.Join(layerTexts, "; ")),
	}, nil
}

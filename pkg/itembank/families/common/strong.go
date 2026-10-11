package common

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

// Семейства с нулевым угадыванием (short_text, numeric, sql_result): ответ всегда
// вычисляется моделированием той же задачи, которую решает кандидат.
func init() {
	itembank.Register(dfsOrder{})
	itembank.Register(heapArray{})
	itembank.Register(loopIterations{})
	itembank.Register(forkCount{})
	itembank.Register(sqlLeftJoinAggregate{})
}

// dfsOrder — порядок посещения вершин при обходе небольшого графа в глубину или в ширину
// по явно заданным спискам смежности; ответ вычисляется моделированием обхода.
type dfsOrder struct{}

func (dfsOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.data_structures.dfs-order.v1", Spec: itembank.SpecCommon, Topic: "data_structures",
		Grade: itembank.GradeJunior, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.5, B: -0.6, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"graph", "dfs", "bfs"},
	}
}

// randomAdjacency строит связный неориентированный граф на n вершинах (остовное дерево плюс
// extra дополнительных рёбер) со случайной перенумерацией; списки смежности отсортированы.
func randomAdjacency(r *rand.Rand, n, extra int) [][]int {
	label := r.Perm(n) // внутренний индекс → метка 0…n−1
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
	for i := 1; i < n; i++ {
		add(i, r.IntN(i))
	}
	for ; extra > 0; extra-- {
		for tries := 0; tries < 20; tries++ {
			if add(r.IntN(n), r.IntN(n)) {
				break
			}
		}
	}
	adj := make([][]int, n)
	for _, e := range edges {
		a, b := label[e.a], label[e.b]
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	for _, list := range adj {
		sort.Ints(list)
	}
	return adj
}

// dfsWalk — рекурсивный обход в глубину; order и trace заполняются метками 1…n.
func dfsWalk(adj [][]int, v int, seen []bool, order *[]int, trace *[]string) {
	seen[v] = true
	*order = append(*order, v+1)
	for _, w := range adj[v] {
		if seen[w] {
			continue
		}
		*trace = append(*trace, fmt.Sprintf("→ %d", w+1))
		dfsWalk(adj, w, seen, order, trace)
		*trace = append(*trace, fmt.Sprintf("↩ %d", v+1))
	}
}

// bfsWalk — обход в ширину с очередью; вершина отмечается при добавлении в очередь.
func bfsWalk(adj [][]int, s int) (order []int, trace []string) {
	seen := make([]bool, len(adj))
	seen[s] = true
	queue := []int{s}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		order = append(order, v+1)
		var added []int
		for _, w := range adj[v] {
			if !seen[w] {
				seen[w] = true
				queue = append(queue, w)
				added = append(added, w+1)
			}
		}
		addedText := "ничего"
		if len(added) > 0 {
			addedText = joinInts(added, ", ")
		}
		trace = append(trace, fmt.Sprintf("извлекли %d, добавили %s", v+1, addedText))
	}
	return order, trace
}

func (f dfsOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE30)
	n := itembank.IntBetween(r, 6, 8)
	var adj [][]int
	var s int
	var dfs, bfs []int
	var dfsTrace, bfsTrace []string
	// Подбираем граф, на котором DFS и BFS дают разные последовательности: иначе задание
	// не отличает одного обхода от другого.
	for attempt := 0; attempt < 30; attempt++ {
		adj = randomAdjacency(r, n, itembank.IntBetween(r, 2, 4))
		s = r.IntN(n)
		dfs, dfsTrace = nil, nil
		dfsWalk(adj, s, make([]bool, n), &dfs, &dfsTrace)
		bfs, bfsTrace = bfsWalk(adj, s)
		if !slices.Equal(dfs, bfs) {
			break
		}
	}
	lines := make([]string, n)
	for v, list := range adj {
		labels := make([]int, len(list))
		for i, w := range list {
			labels[i] = w + 1
		}
		lines[v] = fmt.Sprintf("- `%d`: %s", v+1, joinInts(labels, ", "))
	}
	intro := itembank.Pick(r, []string{
		fmt.Sprintf("Неориентированный граф с вершинами 1…%d задан списками смежности; соседи каждой вершины перечислены по возрастанию, "+
			"и именно в этом порядке их просматривает алгоритм:", n),
		fmt.Sprintf("Для неориентированного графа на %d вершинах (1…%d) известны списки соседей; алгоритм перебирает соседей строго "+
			"в указанном порядке (по возрастанию номера):", n, n),
	})
	var question, answer, explain string
	if r.IntN(2) == 0 {
		question = itembank.Pick(r, []string{
			fmt.Sprintf("Из вершины %d запускают рекурсивный обход в глубину (DFS): вершина отмечается посещённой при входе в неё, затем алгоритм "+
				"переходит к её первому непосещённому соседу. Запишите вершины в порядке их первого посещения через пробел.", s+1),
			fmt.Sprintf("Выполняется обход в глубину (DFS, рекурсивно) начиная с вершины %d. В каком порядке будут посещены вершины? "+
				"Ответ — номера вершин через пробел.", s+1),
		})
		answer = joinInts(dfs, " ")
		explain = fmt.Sprintf("Ход обхода (→ — переход к непосещённому соседу, ↩ — возврат после исчерпания соседей): %s. Порядок посещения: %s. "+
			"Для сравнения, обход в ширину дал бы %s. Типичная ошибка — брать соседей не в указанном порядке, повторно заходить в уже посещённую "+
			"вершину или перепутать DFS с BFS, перечисляя всех соседей текущей вершины подряд.",
			strconv.Itoa(s+1)+" "+strings.Join(dfsTrace, " "), answer, joinInts(bfs, " "))
	} else {
		question = itembank.Pick(r, []string{
			fmt.Sprintf("Из вершины %d запускают обход в ширину (BFS) с очередью: вершина считается посещённой, когда её извлекают из очереди; "+
				"её ещё не добавленные соседи ставятся в очередь в порядке списка. Запишите вершины в порядке посещения через пробел.", s+1),
			fmt.Sprintf("Выполняется обход в ширину (BFS) начиная с вершины %d; стартовая вершина помещается в очередь первой, каждая вершина "+
				"добавляется в очередь не более одного раза. В каком порядке вершины будут извлечены из очереди? Ответ — номера вершин через пробел.", s+1),
		})
		answer = joinInts(bfs, " ")
		explain = fmt.Sprintf("Работа очереди: %s. Порядок посещения: %s. Для сравнения, обход в глубину дал бы %s. Типичная ошибка — ставить в очередь "+
			"уже добавленную вершину, обходить соседей не по порядку списка или перепутать BFS с DFS, уходя вглубь к первому соседу.",
			strings.Join(bfsTrace, "; "), answer, joinInts(dfs, " "))
	}
	return itembank.Instance{
		FamilyID:    f.Meta().ID,
		Seed:        seed,
		Stem:        intro + "\n\n" + strings.Join(lines, "\n") + "\n\n" + question,
		Answer:      itembank.Answer{Text: answer},
		Explanation: explain,
	}, nil
}

// heapArray — содержимое массива двоичной кучи после вставок, извлечений или build-heap;
// ответ вычисляется моделированием просеиваний.
type heapArray struct{}

func (heapArray) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.data_structures.heap-array.v1", Spec: itembank.SpecCommon, Topic: "data_structures",
		Grade: itembank.GradeMiddle, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.6, B: 0.3, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"heap", "priority-queue"},
	}
}

// binHeap — двоичная куча в массиве: корень в индексе 0, дети элемента i — в 2i+1 и 2i+2.
type binHeap struct {
	a     []int
	isMin bool
}

// higher — элемент i должен стоять выше элемента j (ключи различны, ничьих нет).
func (h *binHeap) higher(i, j int) bool {
	if h.isMin {
		return h.a[i] < h.a[j]
	}
	return h.a[i] > h.a[j]
}

func (h *binHeap) push(v int) {
	h.a = append(h.a, v)
	i := len(h.a) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !h.higher(i, p) {
			break
		}
		h.a[i], h.a[p] = h.a[p], h.a[i]
		i = p
	}
}

func (h *binHeap) siftDown(i int) {
	for {
		best := i
		if l := 2*i + 1; l < len(h.a) && h.higher(l, best) {
			best = l
		}
		if rt := 2*i + 2; rt < len(h.a) && h.higher(rt, best) {
			best = rt
		}
		if best == i {
			return
		}
		h.a[i], h.a[best] = h.a[best], h.a[i]
		i = best
	}
}

// pop удаляет корень: на его место ставится последний элемент и просеивается вниз.
func (h *binHeap) pop() int {
	top := h.a[0]
	last := len(h.a) - 1
	h.a[0] = h.a[last]
	h.a = h.a[:last]
	if last > 0 {
		h.siftDown(0)
	}
	return top
}

// build — процедура Флойда: просеивание вниз от последнего внутреннего узла к корню.
func (h *binHeap) build(vals []int) {
	h.a = slices.Clone(vals)
	for i := len(h.a)/2 - 1; i >= 0; i-- {
		h.siftDown(i)
	}
}

func (f heapArray) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE31)
	h := &binHeap{isMin: r.IntN(3) != 0}
	kind, better, extract := "min", "меньший", "extract-min"
	if !h.isMin {
		kind, better, extract = "max", "больший", "extract-max"
	}
	n := itembank.IntBetween(r, 6, 8)
	keys := distinctInts(r, n, 1, 99)
	mode := r.IntN(3) // 0 — вставки; 1 — вставки и извлечения; 2 — build-heap (и, возможно, извлечение)
	var steps []string
	var intro string
	var built []int // результат build-heap до извлечений
	pops := 0
	if mode < 2 {
		for _, k := range keys {
			h.push(k)
			steps = append(steps, fmt.Sprintf("+%d → [%s]", k, joinInts(h.a, " ")))
		}
		intro = fmt.Sprintf("В пустую %s-кучу по очереди добавляют ключи %s (после каждой вставки новый элемент просеивается вверх, "+
			"пока не займёт своё место).", kind, joinInts(keys, ", "))
		if mode == 1 {
			pops = itembank.IntBetween(r, 1, 2)
		}
	} else {
		h.build(keys)
		built = slices.Clone(h.a)
		steps = append(steps, fmt.Sprintf("build-heap → [%s]", joinInts(h.a, " ")))
		intro = fmt.Sprintf("Массив [%s] превращают в %s-кучу процедурой build-heap: элементы с индексами от ⌊n/2⌋ − 1 до 0 по очереди "+
			"просеиваются вниз (без предварительной сортировки и без повторных вставок).", joinInts(keys, ", "), kind)
		pops = r.IntN(2)
	}
	for i := 0; i < pops; i++ {
		top := h.pop()
		steps = append(steps, fmt.Sprintf("%s = %d → [%s]", extract, top, joinInts(h.a, " ")))
	}
	popText := ""
	if pops > 0 {
		popText = fmt.Sprintf(" Затем %d %s выполняют `%s`: корень удаляется, на его место ставится последний элемент массива и просеивается "+
			"вниз (из двух детей выбирается %s).", pops, pluralRu(pops, "раз", "раза", "раз"), extract, better)
	}
	layout := itembank.Pick(r, []string{
		fmt.Sprintf("Двоичная %s-куча хранится в массиве: корень — в индексе 0, дети элемента с индексом i — в индексах 2i + 1 и 2i + 2.", kind),
		fmt.Sprintf("Двоичная куча (%s-heap) реализована на массиве с индексацией от нуля: родитель элемента i находится в индексе ⌊(i − 1)/2⌋.", kind),
	})
	question := itembank.Pick(r, []string{
		"Запишите содержимое массива после всех операций — числа через пробел, начиная с индекса 0.",
		"Каким будет массив кучи после выполнения всех операций? Ответ — элементы через пробел в порядке индексов.",
	})
	answer := joinInts(h.a, " ")
	explain := fmt.Sprintf("Состояние массива после каждого шага: %s. Итог: [%s].", strings.Join(steps, "; "), answer)
	switch {
	case mode == 2:
		alt := &binHeap{isMin: h.isMin}
		for _, k := range keys {
			alt.push(k)
		}
		if slices.Equal(alt.a, built) {
			explain += " Здесь последовательные вставки тех же ключей дали бы такой же массив, но в общем случае build-heap (просеивание вниз " +
				"только внутренних узлов) и цепочка вставок (просеивание вверх каждого нового элемента) строят разные кучи."
		} else {
			explain += fmt.Sprintf(" build-heap просеивает вниз только внутренние узлы и даёт не тот массив, что последовательные вставки "+
				"тех же ключей (они дали бы до извлечений [%s]).", joinInts(alt.a, " "))
		}
	case pops > 0:
		explain += " При извлечении корня на его место ставится последний элемент массива, который затем просеивается вниз; типичная ошибка — " +
			"поднимать на место корня меньшего из детей и сдвигать остальные элементы."
	default:
		explain += " Куча — не отсортированный массив: при вставке элемент меняется местами только с родителем, пока нарушено свойство кучи; " +
			"типичная ошибка — записывать отсортированную последовательность или вставлять без просеивания."
	}
	explain += " Свойство кучи задаёт только отношение «родитель — дети»; порядок между братьями и между поддеревьями не определён."
	return itembank.Instance{
		FamilyID:    f.Meta().ID,
		Seed:        seed,
		Stem:        layout + " " + intro + popText + " " + question,
		Answer:      itembank.Answer{Text: answer},
		Explanation: explain,
	}, nil
}

// loopIterations — точное число срабатываний счётчика во вложенных циклах при конкретном n;
// ответ вычисляется прямым прогоном той же структуры циклов в Go.
type loopIterations struct{}

func (loopIterations) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.complexity.loop-iterations.v1", Spec: itembank.SpecCommon, Topic: "complexity",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: -0.5, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"loops", "counting"},
	}
}

// loopShapeExact — структура циклов: count — оракул, прогоняющий те же циклы, что и показанный
// код; тела — шаблоны с отступом тела функции, %[1]s — имя счётчика.
type loopShapeExact struct {
	minN, maxN             int
	count                  func(n int) int
	goBody, pyBody, jsBody string
	explain                func(n, cnt int) string
}

// loopTerms — слагаемые суммы для пояснения: f(0) + f(1) + … + f(n−1).
func loopTerms(n int, f func(i int) int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = strconv.Itoa(f(i))
	}
	return strings.Join(parts, " + ")
}

var loopShapesExact = []loopShapeExact{
	{
		minN: 6, maxN: 12,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for j := 0; j < i; j++ {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor j := 0; j < i; j++ {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        for j in range(i):\n            %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let j = 0; j < i; j++) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Для i = 0…%d внутренний цикл выполняется i раз: 0 + 1 + … + %d = n(n − 1)/2 = %d. Типичная ошибка — считать n² "+
				"(«два вложенных цикла») или n(n + 1)/2, не заметив строгое неравенство j < i.", n-1, n-1, cnt)
		},
	},
	{
		minN: 6, maxN: 12,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for j := 0; j <= i; j++ {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor j := 0; j <= i; j++ {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        for j in range(i + 1):\n            %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let j = 0; j <= i; j++) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Для i = 0…%d внутренний цикл выполняется i + 1 раз: 1 + 2 + … + %d = n(n + 1)/2 = %d. Типичная ошибка — n(n − 1)/2 "+
				"из-за потерянной итерации j = i или n².", n-1, n, cnt)
		},
	},
	{
		minN: 6, maxN: 12,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for j := i + 1; j < n; j++ {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor j := i + 1; j < n; j++ {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        for j in range(i + 1, n):\n            %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let j = i + 1; j < n; j++) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Для i = 0…%d внутренний цикл выполняется n − i − 1 раз: %d + %d + … + 0 = n(n − 1)/2 = %d — число пар (i, j) с i < j. "+
				"Типичная ошибка — n² или n(n + 1)/2.", n-1, n-1, n-2, cnt)
		},
	},
	{
		minN: 6, maxN: 12,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for j := i; j < n; j++ {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor j := i; j < n; j++ {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        for j in range(i, n):\n            %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let j = i; j < n; j++) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Для i = 0…%d внутренний цикл выполняется n − i раз: %d + %d + … + 1 = n(n + 1)/2 = %d. Типичная ошибка — n(n − 1)/2 "+
				"из-за пропущенной итерации j = i.", n-1, n, n-1, cnt)
		},
	},
	{
		minN: 6, maxN: 13,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i += 2 {
				for j := 0; j < n; j++ {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i += 2 {\n\t\tfor j := 0; j < n; j++ {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(0, n, 2):\n        for j in range(n):\n            %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i += 2) {\n    for (let j = 0; j < n; j++) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			outer := (n + 1) / 2
			return fmt.Sprintf("Внешний цикл выполняется для i = 0, 2, 4, … < %d — ⌈n/2⌉ = %d раз, внутренний при каждом — n = %d раз: %d · %d = %d. "+
				"Типичная ошибка — делить n² на 2 без округления вверх при нечётном n.", n, outer, n, outer, n, cnt)
		},
	},
	{
		minN: 6, maxN: 9,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for j := i; j < n; j += 3 {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor j := i; j < n; j += 3 {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        for j in range(i, n, 3):\n            %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let j = i; j < n; j += 3) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Для i = 0…%d внутренний цикл выполняется ⌈(n − i)/3⌉ раз: %s = %d. Типичная ошибка — делить n(n + 1)/2 на 3 "+
				"вместо округления вверх каждого слагаемого.", n-1, loopTerms(n, func(i int) int { return (n - i + 2) / 3 }), cnt)
		},
	},
	{
		minN: 5, maxN: 20,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for k := 1; k < n; k *= 2 {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor k := 1; k < n; k *= 2 {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        k = 1\n        while k < n:\n            %[1]s += 1\n            k *= 2",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let k = 1; k < n; k *= 2) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			var ks []int
			for k := 1; k < n; k *= 2 {
				ks = append(ks, k)
			}
			return fmt.Sprintf("Внутренний цикл удваивает k от 1, пока k < %d: k принимает значения %s — ⌈log₂ n⌉ = %d раз независимо от i; внешний "+
				"цикл — n = %d раз: %d · %d = %d. Типичная ошибка — брать ⌊log₂ n⌋ или считать внутренний цикл линейным по n.",
				n, joinInts(ks, ", "), len(ks), n, n, len(ks), cnt)
		},
	},
	{
		minN: 5, maxN: 9,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for j := 0; j < i; j++ {
					for k := 0; k < j; k++ {
						c++
					}
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor j := 0; j < i; j++ {\n\t\t\tfor k := 0; k < j; k++ {\n\t\t\t\t%[1]s++\n\t\t\t}\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        for j in range(i):\n            for k in range(j):\n                %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let j = 0; j < i; j++) {\n      for (let k = 0; k < j; k++) {\n        %[1]s++;\n      }\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Счётчик срабатывает по одному разу для каждой тройки k < j < i из множества 0…%d: C(n, 3) = n(n − 1)(n − 2)/6 = "+
				"%d · %d · %d / 6 = %d. Типичная ошибка — n³ или n³/6 без учёта строгих неравенств.", n-1, n, n-1, n-2, cnt)
		},
	},
	{
		minN: 6, maxN: 12,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for j := 0; j < n-i; j++ {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor j := 0; j < n-i; j++ {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        for j in range(n - i):\n            %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let j = 0; j < n - i; j++) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Для i = 0…%d внутренний цикл выполняется n − i раз: %d + %d + … + 1 = n(n + 1)/2 = %d. Типичная ошибка — n(n − 1)/2 "+
				"(первое слагаемое при i = 0 равно n, а не n − 1) или n².", n-1, n, n-1, cnt)
		},
	},
	{
		minN: 6, maxN: 12,
		count: func(n int) int {
			c := 0
			for i := 1; i <= n; i++ {
				for j := 1; j <= i; j++ {
					c++
				}
			}
			return c
		},
		goBody: "\tfor i := 1; i <= n; i++ {\n\t\tfor j := 1; j <= i; j++ {\n\t\t\t%[1]s++\n\t\t}\n\t}",
		pyBody: "    for i in range(1, n + 1):\n        for j in range(1, i + 1):\n            %[1]s += 1",
		jsBody: "  for (let i = 1; i <= n; i++) {\n    for (let j = 1; j <= i; j++) {\n      %[1]s++;\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Для i = 1…%d внутренний цикл выполняется i раз: 1 + 2 + … + %d = n(n + 1)/2 = %d. Типичная ошибка — n(n − 1)/2 "+
				"по привычке к нулевой индексации или n².", n, n, cnt)
		},
	},
	{
		minN: 5, maxN: 10,
		count: func(n int) int {
			c := 0
			for i := 0; i < n; i++ {
				for j := 0; j < n; j++ {
					if i != j {
						c++
					}
				}
			}
			return c
		},
		goBody: "\tfor i := 0; i < n; i++ {\n\t\tfor j := 0; j < n; j++ {\n\t\t\tif i != j {\n\t\t\t\t%[1]s++\n\t\t\t}\n\t\t}\n\t}",
		pyBody: "    for i in range(n):\n        for j in range(n):\n            if i != j:\n                %[1]s += 1",
		jsBody: "  for (let i = 0; i < n; i++) {\n    for (let j = 0; j < n; j++) {\n      if (i !== j) {\n        %[1]s++;\n      }\n    }\n  }",
		explain: func(n, cnt int) string {
			return fmt.Sprintf("Из n² = %d упорядоченных пар (i, j) условие отсекает n = %d пар с i = j: n² − n = n(n − 1) = %d. Типичная ошибка — "+
				"n(n − 1)/2 (неупорядоченные пары, как при j < i) или n².", n*n, n, cnt)
		},
	},
}

func (f loopIterations) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE32)
	shape := itembank.Pick(r, loopShapesExact)
	n := itembank.IntBetween(r, shape.minN, shape.maxN)
	fn := itembank.Ident(r, "func")
	cnt := itembank.Pick(r, []string{"count", "total", "steps", "calls"})
	lang := itembank.Ident(r, "lang")
	var code string
	switch lang {
	case "python":
		code = fmt.Sprintf("def %s(n):\n    %s = 0\n%s\n    return %s", fn, cnt, fmt.Sprintf(shape.pyBody, cnt), cnt)
	case "javascript":
		code = fmt.Sprintf("function %s(n) {\n  let %s = 0;\n%s\n  return %s;\n}", fn, cnt, fmt.Sprintf(shape.jsBody, cnt), cnt)
	default:
		code = fmt.Sprintf("func %s(n int) int {\n\t%s := 0\n%s\n\treturn %s\n}", fn, cnt, fmt.Sprintf(shape.goBody, cnt), cnt)
	}
	answer := shape.count(n)
	call := fmt.Sprintf("%s(%d)", fn, n)
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Функция `%s` приведена в листинге. Какое значение вернёт вызов `%s`?", fn, call),
		fmt.Sprintf("Сколько раз выполнится строка, увеличивающая `%s`, при вызове `%s`? Ответ — число.", cnt, call),
		fmt.Sprintf("Чему будет равна переменная `%s` к моменту `return` при вызове `%s`?", cnt, call),
	})
	return itembank.Instance{
		FamilyID:    f.Meta().ID,
		Seed:        seed,
		Stem:        stem,
		Code:        &itembank.CodeBlock{Lang: lang, Source: code},
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: shape.explain(n, answer),
	}, nil
}

// forkCount — число процессов и вызовов fork() в небольшой программе на C. На границе
// инструкций все живые процессы неразличимы, поэтому модель оперирует их числом.
type forkCount struct{}

func (forkCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.os_processes.fork-count.v1", Spec: itembank.SpecCommon, Topic: "os_processes",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.7, B: 0.4, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"linux", "posix", "fork"},
	}
}

// forkStmt — инструкция тела main. apply по числу живых процессов до неё возвращает число
// живых после и число выполненных вызовов fork(); note — пояснение (%[1]d — процессов до,
// %[2]s — «N процессов» после, %[3]s — «N вызовов»).
type forkStmt struct {
	code     string
	apply    func(alive int) (next, calls int)
	note     string
	usesExit bool
}

var forkStmts = []forkStmt{
	{
		code:  "    fork();",
		apply: func(p int) (int, int) { return 2 * p, p },
		note:  "`fork();` — процессов было %[1]d, каждый делится: %[3]s, стало %[2]s",
	},
	{
		code:  "    if (fork() == 0) {\n        fork();\n    }",
		apply: func(p int) (int, int) { return 3 * p, 2 * p },
		note: "`if (fork() == 0) { fork(); }` — процессов было %[1]d; делится каждый, затем ещё раз делится только потомок (ему fork вернул 0): " +
			"%[3]s, стало %[2]s",
	},
	{
		code:  "    if (fork() > 0) {\n        fork();\n    }",
		apply: func(p int) (int, int) { return 3 * p, 2 * p },
		note: "`if (fork() > 0) { fork(); }` — процессов было %[1]d; делится каждый, затем ещё раз делится только родитель (ему fork вернул pid потомка): " +
			"%[3]s, стало %[2]s",
	},
	{
		code:  "    for (int i = 0; i < 2; i++) {\n        fork();\n    }",
		apply: func(p int) (int, int) { return 4 * p, 3 * p },
		note: "цикл из 2 итераций с fork — процессов было %[1]d; после первой итерации делятся уже обе копии, итого 2² = 4 копии каждого: " +
			"%[3]s, стало %[2]s",
	},
	{
		code:  "    for (int i = 0; i < 3; i++) {\n        fork();\n    }",
		apply: func(p int) (int, int) { return 8 * p, 7 * p },
		note:  "цикл из 3 итераций с fork — процессов было %[1]d; 2³ = 8 копий каждого (а не 3 + 1): %[3]s, стало %[2]s",
	},
	{
		code:  "    if (fork() == 0) {\n        exit(0);\n    }",
		apply: func(p int) (int, int) { return p, p },
		note: "`if (fork() == 0) { exit(0); }` — процессов было %[1]d; каждый создаёт потомка, но потомок сразу завершается: " +
			"%[3]s, число работающих — %[2]s",
		usesExit: true,
	},
	{
		code:  "    if (fork() != 0) {\n        exit(0);\n    }",
		apply: func(p int) (int, int) { return p, p },
		note: "`if (fork() != 0) { exit(0); }` — процессов было %[1]d; каждый создаёт потомка и сам завершается (так запускают демоны): " +
			"%[3]s, число работающих — %[2]s",
		usesExit: true,
	},
}

func nProcs(n int) string {
	return fmt.Sprintf("%d %s", n, pluralRu(n, "процесс", "процесса", "процессов"))
}
func nCalls(n int) string {
	return fmt.Sprintf("%d %s", n, pluralRu(n, "вызов", "вызова", "вызовов"))
}

func (f forkCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE33)
	var stmts []forkStmt
	alive, calls := 1, 0
	var notes []string
	// Подбираем 2–3 инструкции так, чтобы итоговые числа оставались обозримыми.
	for attempt := 0; attempt < 50; attempt++ {
		k := itembank.IntBetween(r, 2, 3)
		stmts = stmts[:0]
		notes = notes[:0]
		alive, calls = 1, 0
		for i := 0; i < k; i++ {
			stmts = append(stmts, itembank.Pick(r, forkStmts))
		}
		for _, st := range stmts {
			next, c := st.apply(alive)
			notes = append(notes, fmt.Sprintf(st.note, alive, nProcs(next), nCalls(c)))
			alive, calls = next, calls+c
		}
		if alive <= 27 && calls <= 40 {
			break
		}
	}
	usesExit := false
	codes := make([]string, len(stmts))
	for i, st := range stmts {
		codes[i] = st.code
		usesExit = usesExit || st.usesExit
	}
	includes := []string{"#include <stdio.h>"}
	if usesExit {
		includes = append(includes, "#include <stdlib.h>")
	}
	includes = append(includes, "#include <unistd.h>")
	code := strings.Join(includes, "\n") + "\n\nint main(void) {\n" + strings.Join(codes, "\n") + "\n    printf(\"done\\n\");\n    return 0;\n}"

	var question string
	var answer int
	switch r.IntN(3) {
	case 0:
		question = "Сколько раз будет напечатана строка `done`?"
		answer = alive
	case 1:
		question = "Сколько всего процессов (включая исходный) будет создано за время работы программы?"
		answer = 1 + calls
	default:
		question = "Сколько дочерних процессов будет создано в сумме, то есть сколько раз выполнится `fork()` по всем процессам?"
		answer = calls
	}
	intro := itembank.Pick(r, []string{
		"Программу на C из листинга запускают в Linux: все вызовы `fork()` успешны, потомков никто не ожидает, каждый `printf` печатает свою строку целиком.",
		"В Linux выполняют программу из листинга (вывод — в терминал построчно, `fork()` всегда успешен, ошибок нет).",
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID,
		Seed:     seed,
		Stem:     intro + " " + question,
		Code:     &itembank.CodeBlock{Lang: "c", Source: code},
		Answer:   itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("В начале один процесс; далее по инструкциям: %s. В конце живых процессов %d — столько строк `done` будет напечатано; "+
			"`fork()` выполнен %s (столько же создано потомков), вместе с исходным — %s. Типичная ошибка — считать, что `fork()` в цикле из k итераций "+
			"даёт k + 1 процесс, что в ветке `== 0` делится родитель, или забывать, что завершившийся через exit процесс до конца main не доходит.",
			strings.Join(notes, "; "), alive, nCalls(calls), nProcs(1+calls)),
	}, nil
}

// sqlLeftJoinAggregate — LEFT JOIN с GROUP BY и агрегатами при NULL: COUNT(*) против COUNT(столбец),
// SUM по пустой группе, фильтр в ON против WHERE, HAVING по нулевому счётчику, INNER JOIN.
type sqlLeftJoinAggregate struct{}

func (sqlLeftJoinAggregate) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.sql.left-join-aggregate.v1", Spec: itembank.SpecCommon, Topic: "sql",
		Grade: itembank.GradeSenior, Format: itembank.FormatSQLResult,
		IRT: itembank.IRTParams{A: 1.6, B: 0.9, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"sql", "postgres"},
	}
}

type joinAggSchema struct {
	parent, child, fk, amount string
	names                     []string
}

var joinAggSchemas = []joinAggSchema{
	{"departments", "employees", "dept_id", "salary", []string{"dev", "ops", "sales", "hr", "legal", "support", "finance", "qa"}},
	{"categories", "products", "category_id", "price", []string{"books", "toys", "food", "tools", "garden", "sport", "music", "games"}},
	{"teams", "tickets", "team_id", "hours", []string{"alpha", "beta", "gamma", "delta", "omega", "sigma", "kappa", "zeta"}},
	{"regions", "orders", "region_id", "amount", []string{"north", "south", "east", "west", "center", "volga", "ural", "siberia"}},
	{"projects", "tasks", "project_id", "estimate", []string{"atlas", "orion", "vega", "nova", "pulsar", "comet", "lyra", "draco"}},
}

func (f sqlLeftJoinAggregate) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE34)
	itoa := strconv.Itoa
	sc := itembank.Pick(r, joinAggSchemas)
	pa, ca := sc.parent[:1], sc.child[:1]
	if pa == ca {
		ca = sc.child[:2]
	}
	np := itembank.IntBetween(r, 3, 4)
	names := itembank.PickN(r, sc.names, np)
	nc := itembank.IntBetween(r, 5, 7)
	empty := itembank.IntBetween(r, 1, np) // родитель, у которого заведомо нет потомков

	type childRow struct{ id, fk, amount int } // fk 0 — NULL, amount −1 — NULL
	children := make([]childRow, nc)
	for i := range children {
		fk := itembank.IntBetween(r, 1, np)
		for fk == empty {
			fk = itembank.IntBetween(r, 1, np)
		}
		children[i] = childRow{i + 1, fk, itembank.IntBetween(r, 1, 9) * 10}
	}
	// 1–2 NULL в сумме, 0–1 строка с NULL во внешнем ключе.
	perm := r.Perm(nc)
	nullAmounts := itembank.IntBetween(r, 1, 2)
	for _, i := range perm[:nullAmounts] {
		children[i].amount = -1
	}
	orphan := -1
	if r.IntN(3) == 0 {
		orphan = perm[nullAmounts]
		children[orphan].fk = 0
	}
	var amounts []int
	for _, c := range children {
		if c.fk > 0 && c.amount >= 0 {
			amounts = append(amounts, c.amount)
		}
	}
	sort.Ints(amounts)
	amounts = slices.Compact(amounts)
	threshold := 0
	switch {
	case len(amounts) >= 2:
		threshold = itembank.Pick(r, amounts[:len(amounts)-1]) // строгое > отсекает хотя бы одно значение, равное порогу
	case len(amounts) == 1:
		threshold = amounts[0] - 10
	}

	// kind: 0 — LEFT JOIN; 1 — фильтр в ON; 2 — фильтр в WHERE; 3 — HAVING cnt = 0; 4 — INNER JOIN.
	kind := r.IntN(5)
	cntKind := r.IntN(3)
	cntExpr := []string{"COUNT(*)", fmt.Sprintf("COUNT(%s.id)", ca), fmt.Sprintf("COUNT(%s.%s)", ca, sc.amount)}[cntKind]
	coalesce := r.IntN(3) == 0
	sumExpr := fmt.Sprintf("SUM(%s.%s)", ca, sc.amount)
	if coalesce {
		sumExpr = "COALESCE(" + sumExpr + ", 0)"
	}
	filtered := kind == 1 || kind == 2

	order := make([]int, np)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return names[order[a]] < names[order[b]] })
	result := [][]string{}
	var groups, dropped []string
	for _, pi := range order {
		id, name := pi+1, names[pi]
		var all, matched []childRow
		for _, c := range children {
			if c.fk != id {
				continue
			}
			all = append(all, c)
			if filtered && (c.amount < 0 || c.amount <= threshold) {
				continue
			}
			matched = append(matched, c)
		}
		rows := max(len(matched), 1) // LEFT JOIN без пары даёт одну строку с NULL
		cntID, cntAmt, sum := len(matched), 0, 0
		for _, c := range matched {
			if c.amount >= 0 {
				cntAmt++
				sum += c.amount
			}
		}
		total := "NULL"
		switch {
		case cntAmt > 0:
			total = itoa(sum)
		case coalesce:
			total = "0"
		}
		cnt := []int{rows, cntID, cntAmt}[cntKind]

		desc := name + ": "
		if len(all) == 0 {
			desc += "потомков нет"
		} else {
			parts := make([]string, len(all))
			for i, c := range all {
				a := "NULL"
				if c.amount >= 0 {
					a = itoa(c.amount)
				}
				parts[i] = fmt.Sprintf("%d (%s)", c.id, a)
			}
			desc += "строки " + strings.Join(parts, ", ")
			if filtered {
				desc += fmt.Sprintf(", фильтр > %d проходят %d", threshold, len(matched))
			}
		}
		switch {
		case (kind == 2 || kind == 4) && len(matched) == 0:
			dropped = append(dropped, name)
			desc += " → группы нет"
			groups = append(groups, desc)
			continue
		case kind == 3 && cnt != 0:
			dropped = append(dropped, name)
			desc += fmt.Sprintf(" → %s = %d, HAVING не пройден", cntExpr, cnt)
			groups = append(groups, desc)
			continue
		}
		desc += fmt.Sprintf(" → COUNT(*) %d, COUNT(%s.id) %d, COUNT(%s.%s) %d, SUM %s", rows, ca, cntID, ca, sc.amount, cntAmt, total)
		groups = append(groups, desc)
		result = append(result, []string{name, itoa(cnt), total})
	}

	joinWord := "LEFT JOIN"
	if kind == 4 {
		joinWord = "JOIN"
	}
	on := fmt.Sprintf("%s.%s = %s.id", ca, sc.fk, pa)
	if kind == 1 {
		on += fmt.Sprintf(" AND %s.%s > %d", ca, sc.amount, threshold)
	}
	lines := []string{
		fmt.Sprintf("SELECT %s.name, %s AS cnt, %s AS total", pa, cntExpr, sumExpr),
		fmt.Sprintf("FROM %s %s", sc.parent, pa),
		fmt.Sprintf("%s %s %s ON %s", joinWord, sc.child, ca, on),
	}
	if kind == 2 {
		lines = append(lines, fmt.Sprintf("WHERE %s.%s > %d", ca, sc.amount, threshold))
	}
	lines = append(lines, fmt.Sprintf("GROUP BY %s.name", pa))
	if kind == 3 {
		lines = append(lines, fmt.Sprintf("HAVING %s = 0", cntExpr))
	}
	lines = append(lines, fmt.Sprintf("ORDER BY %s.name;", pa))

	parentRows := make([][]string, np)
	for i, name := range names {
		parentRows[i] = []string{itoa(i + 1), name}
	}
	childRows := make([][]string, nc)
	for i, c := range children {
		fkCell, amtCell := "NULL", "NULL"
		if c.fk > 0 {
			fkCell = itoa(c.fk)
		}
		if c.amount >= 0 {
			amtCell = itoa(c.amount)
		}
		childRows[i] = []string{itoa(c.id), fkCell, amtCell}
	}

	var trap string
	switch kind {
	case 0:
		trap = fmt.Sprintf("LEFT JOIN сохраняет родителя без потомков как одну строку с NULL в столбцах `%s`: COUNT(*) считает эту строку (1), "+
			"COUNT(%s.id) и COUNT(%s.%s) — нет (0), SUM по группе без значений — NULL (COALESCE превращает его в 0). SUM игнорирует NULL в %s, "+
			"а COUNT(%s.id) такие строки считает.", sc.child, ca, ca, sc.amount, sc.amount, ca)
	case 1:
		trap = fmt.Sprintf("Условие в ON отбирает строки `%s` до соединения: родители, у которых ни один потомок не прошёл фильтр (NULL > %d даёт "+
			"UNKNOWN, равное порогу значение не проходит строгое >), остаются в результате с NULL-строкой. Типичная ошибка — считать фильтр в ON "+
			"эквивалентным фильтру в WHERE.", sc.child, threshold)
	case 2:
		trap = fmt.Sprintf("Фильтр в WHERE применяется после соединения: у NULL-строки %s = NULL, сравнение даёт UNKNOWN, строка отбрасывается — LEFT JOIN "+
			"вырождается во внутреннее соединение, и группы без подходящих потомков (%s) исчезают. Чтобы сохранить их, условие нужно перенести в ON.",
			sc.amount, strings.Join(dropped, ", "))
	case 3:
		switch cntKind {
		case 0:
			trap = "COUNT(*) у родителя без потомков равен 1 (NULL-строка тоже строка), а не 0, поэтому HAVING COUNT(*) = 0 не выполняется ни для одной " +
				fmt.Sprintf("группы и результат пуст. Чтобы найти родителей без потомков, нужен COUNT(%s.id) = 0.", ca)
		case 1:
			trap = fmt.Sprintf("COUNT(%s.id) не считает NULL-строку, поэтому HAVING COUNT(%s.id) = 0 оставляет только родителей без потомков; COUNT(*) дал бы "+
				"для них 1 и пустой результат.", ca, ca)
		default:
			trap = fmt.Sprintf("COUNT(%s.%s) считает только строки с не-NULL значением, поэтому нулю он равен и у родителей без потомков, и у тех, чьи потомки "+
				"все с NULL в %s.", ca, sc.amount, sc.amount)
		}
	default:
		trap = fmt.Sprintf("Внутреннее соединение отбрасывает родителей без потомков (%s): строк с NULL справа здесь не возникает, а SUM всё равно "+
			"игнорирует NULL в %s и для группы из одних NULL даёт NULL.", strings.Join(dropped, ", "), sc.amount)
	}
	orphanNote := ""
	if orphan >= 0 {
		orphanNote = fmt.Sprintf(" Строка `%s` с id %d имеет NULL в %s: она не соединяется ни с одним родителем и на результат не влияет.",
			sc.child, children[orphan].id, sc.fk)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID,
		Seed:     seed,
		Stem: fmt.Sprintf("Таблицы `%s` и `%s` (NULL — отсутствующее значение):\n\n%s\n%s\nЧто вернёт запрос? Укажите строки результата (name, cnt, total) "+
			"в порядке выдачи; NULL записывайте как `NULL`; если результат пуст — оставьте список строк пустым.",
			sc.parent, sc.child, mdTable([]string{"id", "name"}, parentRows), mdTable([]string{"id", sc.fk, sc.amount}, childRows)),
		Code:        &itembank.CodeBlock{Lang: "sql", Source: strings.Join(lines, "\n")},
		Answer:      itembank.Answer{Rows: result},
		Explanation: fmt.Sprintf("Группы по родителям (в порядке ORDER BY name): %s. %s%s", strings.Join(groups, "; "), trap, orphanNote),
	}, nil
}

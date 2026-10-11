package common

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(stackOps{})
	itembank.Register(binarySearchSteps{})
	itembank.Register(subnetHosts{})
}

// stackOps — состояние стека после последовательности операций.
type stackOps struct{}

func (stackOps) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.data_structures.stack-ops.v1", Spec: itembank.SpecCommon, Topic: "data_structures",
		Grade: itembank.GradeJunior, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.0, B: -1.1, C: 0.0}, TimeLimit: 60 * time.Second,
	}
}

func (f stackOps) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE03)
	var stack []int
	var ops []string
	n := itembank.IntBetween(r, 6, 9)
	for i := 0; i < n; i++ {
		// pop разрешён только при непустом стеке; последняя операция — push, чтобы ответ был числом.
		if len(stack) > 0 && i < n-1 && r.IntN(3) == 0 {
			stack = stack[:len(stack)-1]
			ops = append(ops, "pop()")
			continue
		}
		v := itembank.IntBetween(r, 1, 99)
		stack = append(stack, v)
		ops = append(ops, fmt.Sprintf("push(%d)", v))
	}
	top := stack[len(stack)-1]
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: "Над пустым стеком последовательно выполнили операции: `" + strings.Join(ops, "`, `") +
			"`. Какое число находится на вершине стека после всех операций? Ответ — число.",
		Answer: itembank.Answer{Text: strconv.Itoa(top)},
		Explanation: fmt.Sprintf("Стек работает по принципу LIFO: pop снимает последний добавленный элемент. После всех операций в стеке %d элементов, на вершине — %d. "+
			"Типичная ошибка — снимать первый добавленный элемент, как в очереди.", len(stack), top),
	}, nil
}

// binarySearchSteps — число итераций бинарного поиска.
type binarySearchSteps struct{}

func (binarySearchSteps) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.data_structures.binary-search-steps.v1", Spec: itembank.SpecCommon, Topic: "data_structures",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.2, C: 0.0}, TimeLimit: 120 * time.Second,
	}
}

func (f binarySearchSteps) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE04)
	n := itembank.IntBetween(r, 7, 15)
	set := map[int]bool{}
	arr := make([]int, 0, n)
	for len(arr) < n {
		v := itembank.IntBetween(r, 1, 120)
		if !set[v] {
			set[v] = true
			arr = append(arr, v)
		}
	}
	sort.Ints(arr)
	var target int
	present := r.IntN(2) == 0
	if present {
		target = arr[r.IntN(n)]
	} else {
		for {
			target = itembank.IntBetween(r, 1, 120)
			if !set[target] {
				break
			}
		}
	}
	lo, hi, steps := 0, n-1, 0
	var trace []string
	for lo <= hi {
		mid := (lo + hi) / 2
		steps++
		trace = append(trace, fmt.Sprintf("mid=%d (%d)", mid, arr[mid]))
		switch {
		case arr[mid] == target:
			lo = hi + 1 // выход
		case arr[mid] < target:
			lo = mid + 1
		default:
			hi = mid - 1
		}
	}
	strs := make([]string, len(arr))
	for i, v := range arr {
		strs[i] = strconv.Itoa(v)
	}
	outcome := "элемент найден"
	if !present {
		outcome = "элемент отсутствует, поиск завершился, когда левая граница превысила правую"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("В отсортированном массиве `[%s]` бинарным поиском ищут значение %d. Границы включительные, "+
			"`mid = (lo + hi) / 2` с округлением вниз, сравнение с `arr[mid]` — одна итерация. Сколько итераций выполнит алгоритм?",
			strings.Join(strs, ", "), target),
		Answer: itembank.Answer{Number: float64(steps)},
		Explanation: fmt.Sprintf("Последовательность середин: %s; итого %d итераций (%s). Типичная ошибка — считать log₂ n вместо "+
			"прохода по конкретным границам.", strings.Join(trace, " → "), steps, outcome),
	}, nil
}

// subnetHosts — число адресов в подсети по маске.
type subnetHosts struct{}

func (subnetHosts) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.networking.subnet-hosts.v1", Spec: itembank.SpecCommon, Topic: "networking",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.0, B: -0.6, C: 0.0}, TimeLimit: 60 * time.Second,
	}
}

func (f subnetHosts) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE05)
	prefix := itembank.IntBetween(r, 20, 29)
	hostBits := 32 - prefix
	total := 1 << hostBits
	var network string
	switch r.IntN(3) {
	case 0:
		network = fmt.Sprintf("10.%d.%d.0", itembank.IntBetween(r, 0, 255), itembank.IntBetween(r, 0, 15)*16)
	case 1:
		network = fmt.Sprintf("192.168.%d.0", itembank.IntBetween(r, 0, 255))
	default:
		network = fmt.Sprintf("172.%d.%d.0", itembank.IntBetween(r, 16, 31), itembank.IntBetween(r, 0, 15)*16)
	}
	askUsable := r.IntN(2) == 0
	answer := total
	question := fmt.Sprintf("Сколько всего IPv4-адресов содержит подсеть `%s/%d`?", network, prefix)
	explain := fmt.Sprintf("Под адреса хостов отведено 32 − %d = %d бит, то есть 2^%d = %d адресов.", prefix, hostBits, hostBits, total)
	if askUsable {
		answer = total - 2
		question = fmt.Sprintf("Сколько адресов в подсети `%s/%d` можно назначить хостам (без адреса сети и широковещательного)?", network, prefix)
		explain += fmt.Sprintf(" Два адреса зарезервированы (сеть и broadcast): %d − 2 = %d.", total, answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        question,
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: explain + " Типичная ошибка — вычитать 2 там, где спрашивают общее число адресов, или забывать про него для хостов.",
	}, nil
}

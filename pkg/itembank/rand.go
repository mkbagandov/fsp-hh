package itembank

import (
	"math/rand/v2"
)

// NewRand возвращает детерминированный генератор для пары (seed, salt).
// salt — константа семейства: один seed даёт разные потоки в разных семействах.
func NewRand(seed, salt uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, salt^0x9E3779B97F4A7C15))
}

// Pick выбирает случайный элемент.
func Pick[T any](r *rand.Rand, xs []T) T {
	return xs[r.IntN(len(xs))]
}

// PickN выбирает n различных элементов (n ≤ len(xs)).
func PickN[T any](r *rand.Rand, xs []T, n int) []T {
	idx := r.Perm(len(xs))
	if n > len(xs) {
		n = len(xs)
	}
	out := make([]T, 0, n)
	for _, i := range idx[:n] {
		out = append(out, xs[i])
	}
	return out
}

// Shuffle возвращает перемешанную копию.
func Shuffle[T any](r *rand.Rand, xs []T) []T {
	out := make([]T, len(xs))
	copy(out, xs)
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// IntBetween возвращает целое в [lo, hi].
func IntBetween(r *rand.Rand, lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + r.IntN(hi-lo+1)
}

// Словари идентификаторов: разные имена таблиц, переменных и сервисов — одна из осей
// вариативности, которая делает экземпляры непохожими друг на друга.
var identDict = map[string][]string{
	"table":    {"orders", "users", "products", "payments", "invoices", "shipments", "sessions", "events", "tickets", "accounts", "customers", "employees", "departments", "projects", "tasks", "reviews", "bookings", "transactions", "subscriptions", "messages"},
	"column":   {"status", "amount", "qty", "price", "total", "score", "level", "rank", "weight", "duration", "balance", "rating", "count", "priority", "age"},
	"var":      {"count", "total", "acc", "buf", "idx", "cur", "prev", "left", "right", "items", "result", "value", "sum", "n", "k", "x", "y", "z", "tmp", "limit"},
	"func":     {"process", "compute", "handle", "build", "render", "parse", "validate", "transform", "merge", "collect", "apply", "resolve", "fetch", "update", "calc"},
	"service":  {"billing", "catalog", "notifier", "gateway", "scheduler", "ledger", "inventory", "auth-service", "search", "ingest", "reports", "payments-api", "profile", "orders-api"},
	"name_lat": {"alice", "bob", "carol", "dave", "erin", "frank", "grace", "heidi", "ivan", "judy", "mallory", "oscar", "peggy", "trent", "victor", "wendy"},
	"name_ru":  {"Анна", "Борис", "Вера", "Глеб", "Дарья", "Егор", "Жанна", "Иван", "Кира", "Лев", "Мария", "Никита", "Ольга", "Пётр", "Роман", "Света"},
	"city":     {"Москва", "Казань", "Самара", "Пермь", "Томск", "Тула", "Уфа", "Омск", "Сочи", "Тверь", "Калуга", "Рязань"},
	"product":  {"ноутбук", "монитор", "клавиатура", "мышь", "кабель", "наушники", "камера", "роутер", "принтер", "планшет", "колонка", "микрофон"},
	"lang":     {"go", "python", "javascript"},
}

// Ident возвращает случайный идентификатор из словаря kind
// (table, column, var, func, service, name_lat, name_ru, city, product).
func Ident(r *rand.Rand, kind string) string {
	xs, ok := identDict[kind]
	if !ok {
		return "x"
	}
	return Pick(r, xs)
}

// Idents возвращает n различных идентификаторов из словаря.
func Idents(r *rand.Rand, kind string, n int) []string {
	xs, ok := identDict[kind]
	if !ok {
		return nil
	}
	return PickN(r, xs, n)
}

var optionKeys = []string{"a", "b", "c", "d", "e", "f"}

// ShuffledOptions строит перемешанный список вариантов и возвращает ключ верного.
// Дистракторы, совпадающие с верным ответом или друг с другом, отбрасываются.
func ShuffledOptions(r *rand.Rand, correct string, distractors ...string) ([]Option, string) {
	texts := []string{correct}
	seen := map[string]bool{correct: true}
	for _, d := range distractors {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		texts = append(texts, d)
	}
	shuffled := Shuffle(r, texts)
	out := make([]Option, 0, len(shuffled))
	correctKey := ""
	for i, t := range shuffled {
		key := optionKeys[i]
		out = append(out, Option{Key: key, Text: t})
		if t == correct {
			correctKey = key
		}
	}
	return out, correctKey
}

// ShuffledMulti — как ShuffledOptions, но для нескольких верных вариантов.
func ShuffledMulti(r *rand.Rand, correct []string, distractors ...string) ([]Option, []string) {
	texts := []string{}
	seen := map[string]bool{}
	isCorrect := map[string]bool{}
	for _, c := range correct {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		isCorrect[c] = true
		texts = append(texts, c)
	}
	for _, d := range distractors {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		texts = append(texts, d)
	}
	shuffled := Shuffle(r, texts)
	out := make([]Option, 0, len(shuffled))
	keys := []string{}
	for i, t := range shuffled {
		key := optionKeys[i]
		out = append(out, Option{Key: key, Text: t})
		if isCorrect[t] {
			keys = append(keys, key)
		}
	}
	return out, keys
}

// OrderingOptions перемешивает элементы, которые нужно упорядочить, и возвращает
// варианты с ключами и верный порядок ключей.
func OrderingOptions(r *rand.Rand, inOrder []string) ([]Option, []string) {
	idx := r.Perm(len(inOrder))
	keyOf := make([]string, len(inOrder))
	out := make([]Option, len(inOrder))
	for pos, origIdx := range idx {
		key := optionKeys[pos]
		out[pos] = Option{Key: key, Text: inOrder[origIdx]}
		keyOf[origIdx] = key
	}
	return out, keyOf
}

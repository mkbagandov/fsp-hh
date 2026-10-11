package backend

import (
	"fmt"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(httpStatusCodes{})
	itembank.Register(isolationAnomaly{})
	itembank.Register(goroutineCounter{})
}

// httpStatusCodes — какой код статуса вернуть в ситуации.
type httpStatusCodes struct{}

func (httpStatusCodes) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.http_rest.status-codes.v1", Spec: itembank.SpecBackend, Topic: "http_rest",
		Grade: itembank.GradeJunior, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: -0.9, C: 0.25}, TimeLimit: 60 * time.Second, Tags: []string{"http"},
	}
}

type statusScenario struct {
	code    string
	stem    func(entity string, id int) string
	explain string
}

var statusScenarios = []statusScenario{
	{"201 Created", func(e string, _ int) string {
		return fmt.Sprintf("Клиент отправил `POST /%s` с валидным телом, сервер создал новую запись и возвращает её представление.", e)
	}, "Успешное создание ресурса обозначается кодом 201 Created; 200 допустим, но менее точен."},
	{"204 No Content", func(e string, id int) string {
		return fmt.Sprintf("Клиент отправил `DELETE /%s/%d`, запись удалена, тело ответа отправлять нечего.", e, id)
	}, "Успех без тела ответа — 204 No Content."},
	{"400 Bad Request", func(e string, _ int) string {
		return fmt.Sprintf("Клиент отправил `POST /%s`, но тело запроса — невалидный JSON (не парсится).", e)
	}, "Синтаксически неверный запрос — 400 Bad Request."},
	{"401 Unauthorized", func(e string, _ int) string {
		return fmt.Sprintf("Клиент запрашивает `GET /%s` без заголовка Authorization, а ресурс требует входа.", e)
	}, "Отсутствие или невалидность учётных данных — 401 Unauthorized (несмотря на название, это именно аутентификация)."},
	{"403 Forbidden", func(e string, id int) string {
		return fmt.Sprintf("Токен клиента валиден, но его роль не позволяет выполнить `DELETE /%s/%d`.", e, id)
	}, "Пользователь известен, но прав нет — 403 Forbidden."},
	{"404 Not Found", func(e string, id int) string {
		return fmt.Sprintf("Клиент запрашивает `GET /%s/%d`, записи с таким идентификатором не существует.", e, id)
	}, "Ресурс не найден — 404 Not Found."},
	{"409 Conflict", func(e string, _ int) string {
		return fmt.Sprintf("Клиент повторно отправляет `POST /%s` с тем же уникальным ключом, и такая запись уже есть.", e)
	}, "Конфликт с текущим состоянием ресурса (дубликат уникального ключа) — 409 Conflict."},
	{"422 Unprocessable Entity", func(e string, _ int) string {
		return fmt.Sprintf("Тело `POST /%s` — корректный JSON, но поле amount отрицательное, что нарушает бизнес-правило.", e)
	}, "Синтаксис верен, но семантика недопустима — 422 Unprocessable Entity."},
	{"429 Too Many Requests", func(e string, _ int) string {
		return fmt.Sprintf("Клиент превысил лимит запросов к `GET /%s` за минуту.", e)
	}, "Превышение лимита — 429 Too Many Requests, обычно с заголовком Retry-After."},
	{"503 Service Unavailable", func(e string, _ int) string {
		return fmt.Sprintf("Сервис `%s` временно остановлен на обслуживание и не может обработать запрос.", e)
	}, "Временная недоступность сервиса — 503 Service Unavailable."},
}

func (f httpStatusCodes) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x4774)
	sc := itembank.Pick(r, statusScenarios)
	entity := itembank.Ident(r, "table")
	id := itembank.IntBetween(r, 100, 9999)
	var distractors []string
	for _, s := range statusScenarios {
		if s.code != sc.code {
			distractors = append(distractors, s.code)
		}
	}
	options, key := itembank.ShuffledOptions(r, sc.code, itembank.PickN(r, distractors, 3)...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        sc.stem(entity, id) + " Какой код статуса HTTP должен вернуть сервер?",
		Options:     options,
		Answer:      itembank.Answer{Keys: []string{key}},
		Explanation: sc.explain + " Коды 4xx описывают ошибку клиента, 5xx — сервера; 2xx различают, возвращается ли тело.",
	}, nil
}

// isolationAnomaly — назвать аномалию параллельного доступа по сценарию.
type isolationAnomaly struct{}

func (isolationAnomaly) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.transactions.isolation-anomaly.v1", Spec: itembank.SpecBackend, Topic: "transactions",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.3, C: 0.25}, TimeLimit: 120 * time.Second, Tags: []string{"sql", "transactions"},
	}
}

type anomaly struct {
	name    string
	stem    func(table, col string, v1, v2 int) string
	explain string
}

var anomalies = []anomaly{
	{"Грязное чтение (dirty read)", func(t, c string, v1, v2 int) string {
		return fmt.Sprintf("T1 обновляет `%s.%s` с %d на %d, но ещё не зафиксировала транзакцию. T2 читает строку и видит %d. Затем T1 делает ROLLBACK.", t, c, v1, v2, v2)
	}, "T2 прочитала незафиксированное изменение, которое потом откатилось — это грязное чтение; его исключает уже READ COMMITTED."},
	{"Неповторяющееся чтение (non-repeatable read)", func(t, c string, v1, v2 int) string {
		return fmt.Sprintf("T1 читает `%s.%s` и получает %d. T2 обновляет это поле до %d и фиксируется. T1 повторяет тот же запрос и получает %d.", t, c, v1, v2, v2)
	}, "Повторное чтение той же строки в одной транзакции вернуло другое зафиксированное значение — неповторяющееся чтение; защищает REPEATABLE READ."},
	{"Фантомное чтение (phantom read)", func(t, c string, v1, _ int) string {
		return fmt.Sprintf("T1 выполняет `SELECT count(*) FROM %s WHERE %s > %d` и получает N. T2 вставляет новую строку с %s = %d и фиксируется. T1 повторяет запрос и получает N + 1.", t, c, v1, c, v1+1)
	}, "Повторный запрос по условию вернул новые строки, которых раньше не было, — фантом; строгая защита — SERIALIZABLE (в PostgreSQL REPEATABLE READ фантомы тоже не показывает)."},
	{"Потерянное обновление (lost update)", func(t, c string, v1, v2 int) string {
		return fmt.Sprintf("T1 и T2 одновременно читают `%s.%s` = %d. T1 записывает %d и фиксируется, затем T2 записывает %d (своё значение, вычисленное из %d) и фиксируется.", t, c, v1, v1+v2, v1-v2, v1)
	}, "Результат T1 перезаписан T2, которая исходила из устаревшего значения, — потерянное обновление; лечится SELECT ... FOR UPDATE, оптимистичными версиями или атомарным UPDATE ... SET x = x + d."},
}

func (f isolationAnomaly) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x4775)
	a := itembank.Pick(r, anomalies)
	table := itembank.Ident(r, "table")
	col := itembank.Pick(r, []string{"balance", "qty", "score", "amount"})
	v1 := itembank.IntBetween(r, 10, 90) * 10
	v2 := itembank.IntBetween(r, 1, 9) * 10
	var distractors []string
	for _, x := range anomalies {
		if x.name != a.name {
			distractors = append(distractors, x.name)
		}
	}
	options, key := itembank.ShuffledOptions(r, a.name, distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        "Две транзакции работают с одной строкой. " + a.stem(table, col, v1, v2) + " Какая аномалия изоляции описана?",
		Options:     options,
		Answer:      itembank.Answer{Keys: []string{key}},
		Explanation: a.explain,
	}, nil
}

// goroutineCounter — что напечатает программа с WaitGroup и мьютексом.
type goroutineCounter struct{}

func (goroutineCounter) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "backend.concurrency.goroutine-counter.v1", Spec: itembank.SpecBackend, Topic: "concurrency",
		Grade: itembank.GradeMiddle, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.2, B: 0.0, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"go"},
	}
}

func (f goroutineCounter) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x4776)
	g := itembank.IntBetween(r, 2, 8)
	k := itembank.IntBetween(r, 5, 40) * 25
	counter := itembank.Pick(r, []string{"counter", "total", "hits", "n"})
	step := itembank.IntBetween(r, 1, 3)
	code := fmt.Sprintf(`package main

import (
	"fmt"
	"sync"
)

func main() {
	var mu sync.Mutex
	var wg sync.WaitGroup
	%s := 0
	for i := 0; i < %d; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < %d; j++ {
				mu.Lock()
				%s += %d
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	fmt.Println(%s)
}`, counter, g, k, counter, step, counter)
	result := g * k * step
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   "Что выведет программа?",
		Code:   &itembank.CodeBlock{Lang: "go", Source: code},
		Answer: itembank.Answer{Text: fmt.Sprintf("%d", result)},
		Explanation: fmt.Sprintf("Мьютекс делает инкремент атомарным, WaitGroup дожидается всех горутин, поэтому результат детерминирован: "+
			"%d горутин × %d итераций × %d = %d. Без мьютекса гонка данных дала бы непредсказуемое меньшее число.", g, k, step, result),
	}, nil
}

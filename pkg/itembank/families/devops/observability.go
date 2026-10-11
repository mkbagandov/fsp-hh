package devops

import (
	"fmt"
	"math"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(errorBudget{})
	itembank.Register(alertThreshold{})
	itembank.Register(promqlCounterRate{})
	itembank.Register(natTable{})
	itembank.Register(dnsTTL{})
}

// ruNum печатает число с запятой в роли десятичного разделителя.
func ruNum(x float64, prec int) string {
	return strings.ReplaceAll(fmt.Sprintf("%.*f", prec, x), ".", ",")
}

// errorBudget — бюджет ошибок в минутах по SLO и окну.
type errorBudget struct{}

func (errorBudget) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.observability.error-budget.v1", Spec: itembank.SpecDevOps, Topic: "observability",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.0, B: -0.7, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"sre", "slo"},
	}
}

var (
	sloLevels = []struct {
		text string
		frac float64 // доля допустимых отказов
	}{{"99", 0.01}, {"99,5", 0.005}, {"99,9", 0.001}, {"99,95", 0.0005}, {"99,99", 0.0001}}
	sloWindows = []struct {
		text string
		days int
	}{{"7 дней", 7}, {"28 дней", 28}, {"30 дней", 30}, {"90 дней", 90}}
)

func (f errorBudget) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F0D)
	svc := itembank.Ident(r, "service")
	slo := itembank.Pick(r, sloLevels)
	win := itembank.Pick(r, sloWindows)
	minutes := win.days * 24 * 60
	budget := float64(minutes) * slo.frac
	formula := fmt.Sprintf("(1 − %s %%) × %d × 24 × 60 мин = %s × %d мин = %s мин", slo.text, win.days, ruNum(slo.frac, 4), minutes, ruNum(budget, 2))
	variant := r.IntN(3)
	if budget < 3 {
		variant = 0
	}
	var stem, explain string
	var answer, tol float64
	switch variant {
	case 0:
		answer, tol = budget, 0.05
		stem = fmt.Sprintf("Для сервиса `%s` установлен SLO доступности %s %% за скользящее окно %s. Сколько минут недоступности допускает "+
			"бюджет ошибок за это окно? Ответ с точностью до десятых.", svc, slo.text, win.text)
		explain = fmt.Sprintf("Бюджет ошибок — доля времени, которую можно «потратить» на отказы: %s. Типичные ошибки — считать в часах или "+
			"брать окно в 31 день при явно указанной длине окна.", formula)
	case 1:
		used := itembank.IntBetween(r, 1, int(budget)-1)
		answer, tol = budget-float64(used), 0.05
		stem = fmt.Sprintf("Для сервиса `%s` установлен SLO доступности %s %% за окно %s. За текущее окно сервис уже был недоступен %d мин. "+
			"Сколько минут бюджета ошибок осталось? Ответ с точностью до десятых.", svc, slo.text, win.text, used)
		explain = fmt.Sprintf("Полный бюджет: %s. Остаток: %s − %d = %s мин. Типичная ошибка — вычитать простой из длины окна, а не из бюджета.",
			formula, ruNum(budget, 2), used, ruNum(answer, 2))
	default:
		used := itembank.IntBetween(r, 1, int(budget)-1)
		answer, tol = float64(used)/budget*100, 0.5
		stem = fmt.Sprintf("Для сервиса `%s` установлен SLO доступности %s %% за окно %s. За текущее окно сервис был недоступен %d мин. "+
			"Какая доля бюджета ошибок (в процентах) израсходована? Ответ с точностью до десятых.", svc, slo.text, win.text, used)
		explain = fmt.Sprintf("Полный бюджет: %s. Израсходовано %d / %s = %s %%. Типичная ошибка — делить простой на длину окна (получится доля "+
			"недоступности, а не доля бюджета).", formula, used, ruNum(budget, 2), ruNum(answer, 1))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: explain,
	}, nil
}

// alertThreshold — для каких сервисов сработает алерт по доле ошибок.
type alertThreshold struct{}

func (alertThreshold) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.observability.alert-threshold.v1", Spec: itembank.SpecDevOps, Topic: "observability",
		Grade: itembank.GradeMiddle, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.0, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"prometheus", "alerting"},
	}
}

func (f alertThreshold) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F0E)
	services := itembank.Idents(r, "service", 5)
	thr := itembank.Pick(r, []struct {
		text string
		bp   int // порог в десятитысячных долях
	}{{"0.01", 100}, {"0.02", 200}, {"0.05", 500}, {"0.1", 1000}})
	totals := make([]int, 5)
	errs := make([]int, 5)
	firing := make([]bool, 5)
	for {
		n := 0
		for i := range services {
			totals[i] = itembank.IntBetween(r, 2, 50) * 1000
			base := float64(totals[i]*thr.bp) / 10000 // ровно пороговое число ошибок
			switch r.IntN(4) {
			case 0:
				errs[i] = int(math.Round(base * float64(itembank.IntBetween(r, 12, 40)) / 10))
			case 1:
				errs[i] = int(base)
			default:
				errs[i] = int(math.Round(base * float64(itembank.IntBetween(r, 2, 8)) / 10))
			}
			firing[i] = errs[i]*10000 > totals[i]*thr.bp // строгое сравнение, как в правиле
			if firing[i] {
				n++
			}
		}
		if n >= 2 && n <= 3 {
			break
		}
	}
	rule := fmt.Sprintf("- alert: HighErrorRate\n  expr: |\n    sum by (service) (rate(http_requests_total{code=~\"5..\"}[5m]))\n"+
		"      / sum by (service) (rate(http_requests_total[5m])) > %s\n  for: 5m\n  labels:\n    severity: page", thr.text)
	var table strings.Builder
	table.WriteString("| Сервис | Запросов всего | Из них 5xx |\n|---|---|---|\n")
	var correct, wrong, details []string
	for i, s := range services {
		fmt.Fprintf(&table, "| `%s` | %d | %d |\n", s, totals[i], errs[i])
		ratio := float64(errs[i]) / float64(totals[i]) * 100
		opt := "`" + s + "`"
		switch {
		case firing[i]:
			correct = append(correct, opt)
			details = append(details, fmt.Sprintf("`%s`: %d/%d = %s %% > %s %% — firing", s, errs[i], totals[i], ruNum(ratio, 2), ruNum(float64(thr.bp)/100, 2)))
		case errs[i]*10000 == totals[i]*thr.bp:
			wrong = append(wrong, opt)
			details = append(details, fmt.Sprintf("`%s`: %d/%d = ровно %s %% — сравнение строгое (`>`), не сработает", s, errs[i], totals[i], ruNum(ratio, 2)))
		default:
			wrong = append(wrong, opt)
			details = append(details, fmt.Sprintf("`%s`: %d/%d = %s %% — ниже порога", s, errs[i], totals[i], ruNum(ratio, 2)))
		}
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Правило алерта приведено ниже. За последние 5 минут сервисы обработали запросы по таблице; это соотношение держится уже "+
			"дольше 5 минут. Для каких сервисов алерт перейдёт в состояние firing? Выберите все подходящие.\n\n%s", table.String()),
		Code:    &itembank.CodeBlock{Lang: "yaml", Source: rule},
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: "Выражение сравнивает долю 5xx среди всех запросов сервиса с порогом; абсолютное число ошибок само по себе ничего не значит. " +
			strings.Join(details, "; ") + ". Типичные ошибки — сравнивать абсолютные числа ошибок и считать равенство порогу срабатыванием.",
	}, nil
}

// promqlCounterRate — increase()/rate() по выборкам счётчика с учётом сброса.
type promqlCounterRate struct{}

func (promqlCounterRate) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.observability.promql-counter-rate.v1", Spec: itembank.SpecDevOps, Topic: "observability",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.9, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"prometheus", "promql"},
	}
}

func (f promqlCounterRate) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F0F)
	metric := itembank.Pick(r, []string{"http_requests_total", "jobs_processed_total", "messages_consumed_total", "cache_misses_total", "login_failures_total"})
	selector := fmt.Sprintf("%s{job=\"%s\"}", metric, itembank.Ident(r, "service"))
	vals := []int{itembank.IntBetween(r, 1000, 90000)}
	reset := -1
	if r.IntN(10) < 6 {
		reset = itembank.IntBetween(r, 2, 4)
	}
	for i := 1; i <= 5; i++ {
		d := itembank.IntBetween(r, 20, 400)
		if i == reset {
			vals = append(vals, d) // счётчик обнулился и успел вырасти до d
		} else {
			vals = append(vals, vals[i-1]+d)
		}
	}
	increase := 0
	var deltas []string
	for i := 1; i <= 5; i++ {
		if vals[i] >= vals[i-1] {
			increase += vals[i] - vals[i-1]
			deltas = append(deltas, fmt.Sprintf("%d", vals[i]-vals[i-1]))
		} else {
			increase += vals[i]
			deltas = append(deltas, fmt.Sprintf("%d (сброс: %d → %d, прирост считается от нуля)", vals[i], vals[i-1], vals[i]))
		}
	}
	naive := vals[5] - vals[0]
	var table strings.Builder
	table.WriteString("| t, с | значение |\n|---|---|\n")
	for i, v := range vals {
		fmt.Fprintf(&table, "| %d | %d |\n", -300+60*i, v)
	}
	askRate := r.IntN(2) == 0
	expr := fmt.Sprintf("increase(%s[5m])", selector)
	answer, tol := float64(increase), 0.5
	precision := "Ответ — целое число."
	explain := fmt.Sprintf("increase() суммирует приросты между соседними выборками, а падение значения трактует как сброс счётчика (рестарт процесса): "+
		"приросты %s, итого %d.", strings.Join(deltas, " + "), increase)
	if askRate {
		expr = fmt.Sprintf("rate(%s[5m])", selector)
		answer, tol = float64(increase)/300, 0.01
		precision = "Ответ с точностью до сотых."
		explain += fmt.Sprintf(" rate() — тот же прирост в пересчёте на секунду: %d / 300 с = %s/с.", increase, ruNum(answer, 3))
	}
	if reset > 0 {
		explain += fmt.Sprintf(" Типичная ошибка — взять разность последнего и первого значений (%d − %d = %d), которая из-за сброса занижает результат.", vals[5], vals[0], naive)
	} else {
		explain += fmt.Sprintf(" Без сбросов результат совпадает с разностью последнего и первого значений (%d − %d = %d); типичная ошибка — делить на число выборок, а не на длину окна в секундах.", vals[5], vals[0], naive)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Счётчик `%s` опрашивается раз в 60 с; выборки за последние 5 минут — в таблице (t — секунды относительно момента вычисления). "+
			"Что вернёт выражение ниже в момент t = 0? Считайте, что в окно попали ровно эти шесть выборок и экстраполяция к границам окна не меняет результат. %s\n\n%s",
			metric, precision, table.String()),
		Code:        &itembank.CodeBlock{Lang: "promql", Source: expr},
		Answer:      itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: explain,
	}, nil
}

// natTable — трансляция адресов по таблице SNAT или правилам DNAT.
type natTable struct{}

func (natTable) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.networking.nat-table.v1", Spec: itembank.SpecDevOps, Topic: "networking",
		Grade: itembank.GradeJunior, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.0, B: -0.6, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"nat", "iptables"},
	}
}

func (f natTable) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F10)
	pub := fmt.Sprintf("203.0.113.%d", itembank.IntBetween(r, 2, 250))
	if r.IntN(3) < 2 {
		hosts := itembank.PickN(r, []int{12, 20, 33, 47, 58, 101}, 2)
		ports := itembank.PickN(r, []int{49152, 50411, 51034, 52780, 54201, 60333, 61999}, 3)
		extBase := 40000 + 1000*r.IntN(20)
		type natRow struct{ internal, external string }
		// Один и тот же внутренний порт у двух хостов и два порта у одного хоста — обе ловушки.
		rows := []natRow{
			{fmt.Sprintf("192.168.1.%d:%d", hosts[0], ports[0]), fmt.Sprintf("%s:%d", pub, extBase+1)},
			{fmt.Sprintf("192.168.1.%d:%d", hosts[1], ports[0]), fmt.Sprintf("%s:%d", pub, extBase+2)},
			{fmt.Sprintf("192.168.1.%d:%d", hosts[0], ports[1]), fmt.Sprintf("%s:%d", pub, extBase+3)},
			{fmt.Sprintf("192.168.1.%d:%d", hosts[1], ports[2]), fmt.Sprintf("%s:%d", pub, extBase+4)},
		}
		rows = itembank.Shuffle(r, rows)
		var table strings.Builder
		table.WriteString("| Источник внутри сети | Источник после трансляции |\n|---|---|\n")
		for _, row := range rows {
			fmt.Fprintf(&table, "| %s | %s |\n", row.internal, row.external)
		}
		pick := itembank.Pick(r, rows)
		var question, answer, explain string
		if r.IntN(2) == 0 {
			question = fmt.Sprintf("Хост отправляет пакет с адреса источника %s на сервер в интернете. Какой адрес источника (в формате `IP:порт`) увидит сервер?", pick.internal)
			answer = pick.external
			explain = fmt.Sprintf("Маршрутизатор с NAT подменяет внутренний адрес источника на свой внешний и запоминает соответствие «внутренний IP:порт ↔ внешний порт», "+
				"чтобы вернуть ответ нужному хосту. Для %s в таблице записан %s. Типичная ошибка — искать строку только по порту (он совпадает у двух хостов) или только по IP.",
				pick.internal, pick.external)
		} else {
			question = fmt.Sprintf("Ответ сервера пришёл на внешний адрес %s. На какой внутренний адрес (в формате `IP:порт`) маршрутизатор перешлёт его?", pick.external)
			answer = pick.internal
			explain = fmt.Sprintf("Для входящего ответа маршрутизатор ищет внешний порт в таблице трансляций и восстанавливает исходный адрес и порт хоста: %s ↔ %s. "+
				"Типичная ошибка — считать, что внутренний порт совпадает с внешним, или перепутать хосты с одинаковым внутренним портом.", pick.external, pick.internal)
		}
		return itembank.Instance{
			FamilyID: f.Meta().ID, Seed: seed,
			Stem:        fmt.Sprintf("Маршрутизатор офиса с внешним адресом %s выполняет SNAT; текущая таблица трансляций ниже. %s\n\n%s", pub, question, table.String()),
			Answer:      itembank.Answer{Text: answer},
			Explanation: explain,
		}, nil
	}
	dports := itembank.PickN(r, []int{8443, 2222, 8080, 9443, 2200, 8000, 10022}, 3)
	internalIPs := itembank.PickN(r, []string{"10.0.0.15", "10.0.0.21", "10.0.0.30", "10.0.0.42"}, 2)
	internalPorts := itembank.PickN(r, []int{443, 22, 80, 8443, 3000, 5432}, 3)
	type dnatRule struct {
		dport  int
		target string
	}
	rules := make([]dnatRule, 0, 3)
	var sb strings.Builder
	for i := 0; i < 3; i++ {
		rule := dnatRule{dports[i], fmt.Sprintf("%s:%d", internalIPs[i%2], internalPorts[i])}
		rules = append(rules, rule)
		fmt.Fprintf(&sb, "iptables -t nat -A PREROUTING -d %s -p tcp --dport %d -j DNAT --to-destination %s\n", pub, rule.dport, rule.target)
	}
	pick := itembank.Pick(r, rules)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("На шлюзе с внешним адресом %s настроены правила ниже. Клиент из интернета устанавливает TCP-соединение на %s:%d. "+
			"На какой адрес и порт (в формате `IP:порт`) попадёт соединение внутри сети?", pub, pub, pick.dport),
		Code:   &itembank.CodeBlock{Lang: "bash", Source: strings.TrimRight(sb.String(), "\n")},
		Answer: itembank.Answer{Text: pick.target},
		Explanation: fmt.Sprintf("Правило DNAT в цепочке PREROUTING подменяет адрес назначения входящего пакета: порт %d на внешнем адресе перенаправляется на %s. "+
			"Внешний и внутренний порты не обязаны совпадать. Типичные ошибки — ожидать тот же порт внутри сети (%d) или перепутать строки с одинаковым внутренним IP.",
			pick.dport, pick.target, pick.dport),
	}, nil
}

// dnsTTL — остаток TTL в цепочке кэшей и худшее время распространения смены адреса.
type dnsTTL struct{}

func (dnsTTL) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.networking.dns-ttl.v1", Spec: itembank.SpecDevOps, Topic: "networking",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.7, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"dns"},
	}
}

func (f dnsTTL) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F11)
	host := fmt.Sprintf("%s.example.com", itembank.Pick(r, []string{"api", "app", "www", "cdn", "auth", "static"}))
	t1 := time.Date(2026, 1, 1, itembank.IntBetween(r, 8, 17), r.IntN(60), r.IntN(60), 0, time.UTC)
	var stem, explain string
	var answer float64
	switch r.IntN(3) {
	case 0:
		ttl := itembank.Pick(r, []int{300, 600, 900, 1800, 3600})
		elapsed := itembank.IntBetween(r, 30, ttl-30)
		t2 := t1.Add(time.Duration(elapsed) * time.Second)
		answer = float64(ttl - elapsed)
		stem = fmt.Sprintf("Авторитативный сервер отдаёт запись `A %s` с TTL %d с. Кэширующий резолвер провайдера получил её в %s. Клиент за этим резолвером "+
			"запросил имя в %s. Какое значение TTL (в секундах) клиент увидит в ответе?", host, ttl, t1.Format("15:04:05"), t2.Format("15:04:05"))
		explain = fmt.Sprintf("Кэширующий резолвер отдаёт запись с оставшимся временем жизни: с момента кэширования прошло %d с, поэтому TTL = %d − %d = %d. "+
			"Типичная ошибка — считать, что резолвер отдаёт исходный TTL %d.", elapsed, ttl, elapsed, ttl-elapsed, ttl)
	case 1:
		ttl := itembank.Pick(r, []int{600, 900, 1800, 3600})
		e2 := itembank.IntBetween(r, 30, ttl/2)
		e3 := itembank.IntBetween(r, e2+30, ttl-30)
		t2, t3 := t1.Add(time.Duration(e2)*time.Second), t1.Add(time.Duration(e3)*time.Second)
		answer = float64(ttl - e3)
		stem = fmt.Sprintf("Запись `A %s` имеет TTL %d с. Резолвер провайдера закэшировал её в %s; корпоративный форвардер (он тоже кэширует ответы) "+
			"запросил её у провайдера в %s; ноутбук сотрудника запросил имя у форвардера в %s. Какой TTL (в секундах) получит ноутбук?",
			host, ttl, t1.Format("15:04:05"), t2.Format("15:04:05"), t3.Format("15:04:05"))
		explain = fmt.Sprintf("Каждый промежуточный кэш отдаёт остаток TTL, а не исходное значение, поэтому отсчёт идёт от первого кэширования в %s: "+
			"%d − %d = %d. Типичная ошибка — считать от момента запроса форвардера (%d − %d = %d): форвардер получил уже уменьшенный TTL %d и продолжил его уменьшать.",
			t1.Format("15:04:05"), ttl, e3, ttl-e3, ttl, e3-e2, ttl-(e3-e2), ttl-e2)
	default:
		oldTTL := itembank.Pick(r, []int{1800, 3600, 7200})
		newTTL := itembank.Pick(r, []int{60, 300})
		gap := itembank.IntBetween(r, 10, 150) // минут между снижением TTL и сменой адреса
		tLow := t1.Truncate(time.Minute)
		tChange := tLow.Add(time.Duration(gap) * time.Minute)
		oldMin, newMin := oldTTL/60, newTTL/60
		latest := gap + newMin
		if oldMin > latest {
			latest = oldMin
		}
		answer = float64(latest - gap)
		stem = fmt.Sprintf("У записи `A %s` TTL был %d с. В %s TTL снизили до %d с, а в %s сменили IP-адрес. Через сколько минут после смены адреса даже самые "+
			"«долгие» кэширующие резолверы гарантированно отдадут новый адрес? Худший случай: резолвер мог закэшировать запись непосредственно перед каждым изменением.",
			host, oldTTL, tLow.Format("15:04"), newTTL, tChange.Format("15:04"))
		explain = fmt.Sprintf("Резолвер, закэшировавший запись перед снижением TTL (%s), держит её до %s (+%d мин); закэшировавший перед сменой адреса (%s) — до %s (+%d мин). "+
			"Позднейший момент — %s, то есть через %d мин после смены адреса. Типичная ошибка — брать только новый TTL (%d мин), хотя старый TTL ещё не истёк, "+
			"или только старый, хотя он уже истёк.", tLow.Format("15:04"), tLow.Add(time.Duration(oldMin)*time.Minute).Format("15:04"), oldMin,
			tChange.Format("15:04"), tChange.Add(time.Duration(newMin)*time.Minute).Format("15:04"), newMin,
			tLow.Add(time.Duration(latest)*time.Minute).Format("15:04"), latest-gap, newMin)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Number: answer},
		Explanation: explain,
	}, nil
}

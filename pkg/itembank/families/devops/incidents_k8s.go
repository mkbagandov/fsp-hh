package devops

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(incidentTimeline{})
	itembank.Register(burnRate{})
	itembank.Register(nodePodFit{})
	itembank.Register(ciJobConditions{})
}

// upperFirst делает первую букву заглавной.
func upperFirst(s string) string {
	rs := []rune(s)
	if len(rs) == 0 {
		return s
	}
	rs[0] = unicode.ToUpper(rs[0])
	return string(rs)
}

// trimNum печатает число с запятой в роли десятичного разделителя без незначащих нулей (до 4 знаков).
func trimNum(x float64) string {
	s := strconv.FormatFloat(x, 'f', 4, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return strings.ReplaceAll(s, ".", ",")
}

// incidentTimeline — метрики реагирования (обнаружение, реакция, восстановление) по журналу инцидента.
type incidentTimeline struct{}

func (incidentTimeline) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.incident_response.timeline-metrics.v1", Spec: itembank.SpecDevOps, Topic: "incident_response",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.4, B: -0.5, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"sre", "incident"},
	}
}

func (f incidentTimeline) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F24)
	svc := itembank.Ident(r, "service")
	ver := fmt.Sprintf("v%d.%d.%d", 1+r.IntN(4), r.IntN(10), 1+r.IntN(9))
	metric, alertName, restoredText := "доли ответов 5xx", "HighErrorRate", "доля 5xx вернулась к норме, сервис восстановлен"
	if r.IntN(2) == 0 {
		metric, alertName, restoredText = "p99-латентности", "HighLatency", "латентность вернулась к норме, сервис восстановлен"
	}
	minutes := func(lo, hi int) time.Duration { return time.Duration(itembank.IntBetween(r, lo, hi)) * time.Minute }
	deploy := clock(r)
	start := deploy.Add(minutes(2, 15))
	alert := start.Add(minutes(2, 12))
	ack := alert.Add(minutes(1, 9))
	rollback := ack.Add(minutes(3, 20))
	restored := rollback.Add(minutes(2, 15))
	closed := restored.Add(minutes(10, 45))
	type event struct {
		at   time.Time
		text string
	}
	events := []event{
		{deploy, itembank.Pick(r, []string{fmt.Sprintf("выкатка `%s` %s завершена", svc, ver), fmt.Sprintf("завершён деплой `%s` %s в production", svc, ver)})},
		{start, fmt.Sprintf("по метрикам начался рост %s `%s`", metric, svc)},
		{alert, fmt.Sprintf("сработал алерт `%s`, уведомление ушло дежурному", alertName)},
		{ack, itembank.Pick(r, []string{"дежурный подтвердил алерт (ack)", "дежурный принял алерт в работу (ack)"})},
		{rollback, fmt.Sprintf("начат откат `%s` на предыдущую версию", svc)},
		{restored, restoredText},
		{closed, "инцидент закрыт, канал заархивирован"},
	}
	if gap := int(rollback.Sub(ack).Minutes()); r.IntN(2) == 0 && gap >= 2 {
		events = append(events, event{ack.Add(time.Duration(itembank.IntBetween(r, 1, gap-1)) * time.Minute), "обновлена статус-страница: «частичная деградация»"})
		sort.Slice(events, func(i, j int) bool { return events[i].at.Before(events[j].at) })
	}
	type metricDef struct {
		name, def        string
		from, to         time.Time
		fromName, toName string
	}
	m := itembank.Pick(r, []metricDef{
		{"время обнаружения", "время обнаружения считается от начала деградации по метрикам до срабатывания алерта", start, alert, "начало роста по метрикам", "срабатывание алерта"},
		{"время реакции", "время реакции считается от срабатывания алерта до его подтверждения дежурным", alert, ack, "срабатывание алерта", "подтверждение дежурным"},
		{"время восстановления", "время восстановления считается от начала деградации по метрикам до момента, когда сервис восстановлен", start, restored, "начало роста по метрикам", "восстановление сервиса"},
		{"время устранения", "время устранения считается от подтверждения алерта дежурным до момента, когда сервис восстановлен", ack, restored, "подтверждение дежурным", "восстановление сервиса"},
	})
	answer := int(m.to.Sub(m.from).Minutes())
	hhmm := func(t time.Time) string { return t.Format("15:04") }
	var table strings.Builder
	table.WriteString("| Время (UTC) | Событие |\n|---|---|\n")
	for _, e := range events {
		fmt.Fprintf(&table, "| %s | %s |\n", hhmm(e.at), e.text)
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Ниже — журнал инцидента по сервису `%s` (одни сутки, события в хронологическом порядке). По принятым в команде определениям %s. Сколько минут составило %s?", svc, m.def, m.name),
		fmt.Sprintf("Дежурный заполняет постмортем по журналу инцидента `%s` (ниже; все события одних суток). Правило команды: %s. Чему равно %s в минутах?", svc, m.def, m.name),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem + "\n\n" + table.String(),
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("%s: %s (%s) − %s (%s) = %d мин. Остальные события журнала (выкатка в %s, начало отката, закрытие инцидента и прочие) "+
			"в это определение не входят. Типичная ошибка — отсчитывать от момента выкатки (причина, но не начало деградации) или до закрытия инцидента вместо указанных в определении событий.",
			upperFirst(m.name), hhmm(m.to), m.toName, hhmm(m.from), m.fromName, answer, hhmm(deploy)),
	}, nil
}

// burnRate — скорость расходования бюджета ошибок и время до его исчерпания.
type burnRate struct{}

func (burnRate) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.incident_response.burn-rate.v1", Spec: itembank.SpecDevOps, Topic: "incident_response",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.6, B: 1.0, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"sre", "slo"},
	}
}

func (f burnRate) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F25)
	svc := itembank.Ident(r, "service")
	slo := itembank.Pick(r, sloLevels)
	win := itembank.Pick(r, sloWindows)
	mult := itembank.Pick(r, []float64{1.5, 2, 3, 4.5, 6, 8, 10, 12, 14.4, 20, 36})
	allowedPct := slo.frac * 100
	errPct := mult * allowedPct
	windowHours := win.days * 24
	exhaust := float64(windowHours) / mult
	hours := func(h float64) string { return trimNum(math.Round(h*10) / 10) }
	intro := fmt.Sprintf("SLO доступности `%s` — %s %% за скользящее окно %s.", svc, slo.text, win.text)
	def := "Burn rate — во сколько раз текущая доля ошибок превышает допустимую по SLO (1 − SLO)."
	base := fmt.Sprintf("Допустимая доля ошибок: 1 − %s %% = %s %%. ", slo.text, trimNum(allowedPct))
	var stem, explain string
	var answer, tol float64
	switch r.IntN(4) {
	case 0:
		answer, tol = mult, 0.05
		stem = fmt.Sprintf("%s За последний час доля ошибок (5xx среди всех запросов) держится на уровне %s %%. %s Чему равен burn rate? Ответ с точностью до десятых.", intro, trimNum(errPct), def)
		explain = base + fmt.Sprintf("Burn rate = %s %% / %s %% = %s. Типичная ошибка — делить долю ошибок на сам SLO (%s %%) вместо допустимой доли или смешивать проценты с долями.",
			trimNum(errPct), trimNum(allowedPct), trimNum(mult), slo.text)
	case 1:
		answer, tol = exhaust, 0.1
		stem = fmt.Sprintf("%s С начала окна ошибок не было, а теперь доля ошибок держится на уровне %s %%. %s Через сколько часов при таком темпе бюджет ошибок за окно будет исчерпан полностью? Ответ с точностью до десятых.",
			intro, trimNum(errPct), def)
		explain = base + fmt.Sprintf("Burn rate = %s / %s = %s. При burn rate B полный бюджет окна сгорает за длину окна / B: %d дней × 24 = %d ч, %d / %s = %s ч. "+
			"Типичные ошибки — считать окно в днях, а не в часах, или делить на SLO вместо 1 − SLO.", trimNum(errPct), trimNum(allowedPct), trimNum(mult), win.days, windowHours, windowHours, trimNum(mult), hours(exhaust))
	case 2:
		remain := itembank.Pick(r, []int{20, 25, 40, 50, 60, 75, 80})
		answer, tol = exhaust*float64(remain)/100, 0.1
		stem = fmt.Sprintf("%s К началу инцидента оставалось %d %% бюджета ошибок за окно; во время инцидента доля ошибок держится на уровне %s %%. %s Через сколько часов при таком темпе остаток бюджета будет исчерпан? Ответ с точностью до десятых.",
			intro, remain, trimNum(errPct), def)
		explain = base + fmt.Sprintf("Burn rate = %s / %s = %s. Полный бюджет окна (%d ч) при таком темпе сгорает за %d / %s = %s ч, а остаток %d %% — за %s × %d %% = %s ч. "+
			"Типичные ошибки — забыть про остаток бюджета или считать окно в днях.", trimNum(errPct), trimNum(allowedPct), trimNum(mult), windowHours, windowHours, trimNum(mult), hours(exhaust), remain, hours(exhaust), remain, hours(answer))
	default:
		answer, tol = errPct, errPct/100
		stem = fmt.Sprintf("%s Правило алерта должно срабатывать, когда burn rate за окно измерения достигает %s. %s Какой порог доли ошибок (в процентах) нужно указать в правиле? Ответ с точностью до тысячных.",
			intro, trimNum(mult), def)
		explain = base + fmt.Sprintf("Порог = burn rate × допустимая доля = %s × %s %% = %s %%. Типичная ошибка — умножать на сам SLO (%s %%) или оставить результат в долях, а не в процентах.",
			trimNum(mult), trimNum(allowedPct), trimNum(errPct), slo.text)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: explain,
	}, nil
}

// nodePodFit — сколько подов поместится на ноду по requests с учётом DaemonSet-ов и нескольких контейнеров.
type nodePodFit struct{}

func (nodePodFit) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.kubernetes.node-pod-fit.v1", Spec: itembank.SpecDevOps, Topic: "kubernetes",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.4, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"kubernetes", "scheduling"},
	}
}

func fmtCPU(milli int) string {
	if milli%1000 == 0 {
		return fmt.Sprintf("\"%d\"", milli/1000)
	}
	return fmt.Sprintf("%dm", milli)
}

func fmtMem(mi int) string {
	if mi%1024 == 0 {
		return fmt.Sprintf("%dGi", mi/1024)
	}
	return fmt.Sprintf("%dMi", mi)
}

func (f nodePodFit) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F26)
	app := itembank.Ident(r, "service")
	var cpuAlloc, memAllocGi, dsCPU, dsMem, appCPU, appMem, sideCPU, sideMem, fitCPU, fitMem, fit int
	var sidecar string
	for {
		cpuAlloc = itembank.Pick(r, []int{1930, 3920, 7910, 15890})
		memAllocGi = itembank.Pick(r, []int{7, 15, 30, 62})
		dsCPU = itembank.Pick(r, []int{150, 250, 350, 450})
		dsMem = itembank.Pick(r, []int{256, 384, 512, 768})
		appCPU = itembank.Pick(r, []int{250, 500, 750, 1000, 1500})
		appMem = itembank.Pick(r, []int{256, 512, 768, 1024, 1536, 2048})
		sidecar, sideCPU, sideMem = "", 0, 0
		if r.IntN(5) < 3 {
			sidecar = itembank.Pick(r, []string{"envoy", "log-agent", "oauth-proxy"})
			sideCPU = itembank.Pick(r, []int{50, 100, 200})
			sideMem = itembank.Pick(r, []int{64, 128, 256})
		}
		fitCPU = (cpuAlloc - dsCPU) / (appCPU + sideCPU)
		fitMem = (memAllocGi*1024 - dsMem) / (appMem + sideMem)
		fit = min(fitCPU, fitMem)
		if fit >= 2 && fit <= 40 && fitCPU != fitMem {
			break
		}
	}
	replicas := itembank.IntBetween(r, fit+1, 3*fit+2)
	var sb strings.Builder
	fmt.Fprintf(&sb, "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: %s\nspec:\n  replicas: %d\n  template:\n    spec:\n      containers:\n", app, replicas)
	fmt.Fprintf(&sb, "      - name: app\n        resources:\n          requests: { cpu: %s, memory: %s }\n          limits: { cpu: %s, memory: %s }\n",
		fmtCPU(appCPU), fmtMem(appMem), fmtCPU(appCPU*2), fmtMem(appMem*2))
	if sidecar != "" {
		fmt.Fprintf(&sb, "      - name: %s\n        resources:\n          requests: { cpu: %s, memory: %s }\n          limits: { cpu: %s, memory: %s }\n",
			sidecar, fmtCPU(sideCPU), fmtMem(sideMem), fmtCPU(sideCPU*2), fmtMem(sideMem*2))
	}
	node := itembank.Pick(r, []string{
		fmt.Sprintf("Нода имеет allocatable `cpu: %dm`, `memory: %dGi` (1 Gi = 1024 Mi). На ней уже работают DaemonSet-поды с суммарными requests `cpu: %dm`, `memory: %dMi`.", cpuAlloc, memAllocGi, dsCPU, dsMem),
		fmt.Sprintf("У каждой ноды кластера allocatable — `cpu: %dm` и `memory: %dGi` (1 Gi = 1024 Mi); DaemonSet-поды на каждой ноде уже заняли requests `cpu: %dm`, `memory: %dMi`.", cpuAlloc, memAllocGi, dsCPU, dsMem),
	})
	rule := "Планировщик размещает под, если сумма requests всех его контейнеров помещается в свободные allocatable-ресурсы ноды; других ограничений нет."
	need := (replicas + fit - 1) / fit
	var question, tail string
	var answer int
	switch r.IntN(3) {
	case 0:
		question = "Сколько подов этого Deployment поместится на одну такую ноду?"
		answer = fit
	case 1:
		question = fmt.Sprintf("Сколько таких нод понадобится, чтобы разместить все %d реплик?", replicas)
		answer = need
		tail = fmt.Sprintf(" Для %d реплик нужно ⌈%d / %d⌉ = %d %s.", replicas, replicas, fit, need, ruPlural(need, "нода", "ноды", "нод"))
	default:
		nodes := itembank.IntBetween(r, 1, need-1)
		answer = replicas - nodes*fit
		question = fmt.Sprintf("В кластере %d %s. Сколько реплик останется в состоянии Pending?", nodes, ruPlural(nodes, "такая нода", "такие ноды", "таких нод"))
		tail = fmt.Sprintf(" На %d %s разместится %d × %d = %d %s, в Pending останется %d − %d = %d.", nodes, ruPlural(nodes, "ноде", "нодах", "нодах"),
			nodes, fit, nodes*fit, ruPlural(nodes*fit, "под", "пода", "подов"), replicas, nodes*fit, answer)
	}
	bottleneck := "CPU"
	if fitMem < fitCPU {
		bottleneck = "память"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   node + " " + rule + " " + question,
		Code:   &itembank.CodeBlock{Lang: "yaml", Source: strings.TrimRight(sb.String(), "\n")},
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Свободно на ноде: cpu %d − %d = %d m, memory %d × 1024 − %d = %d Mi. Requests пода — сумма по контейнерам: cpu %d m, memory %d Mi "+
			"(limits планировщик не учитывает). По CPU помещается ⌊%d / %d⌋ = %d %s, по памяти ⌊%d / %d⌋ = %d; ограничивает %s — %d %s на ноду.%s "+
			"Типичные ошибки — считать по limits, забывать про DaemonSet-ы или второй контейнер и округлять число нод вниз.",
			cpuAlloc, dsCPU, cpuAlloc-dsCPU, memAllocGi, dsMem, memAllocGi*1024-dsMem, appCPU+sideCPU, appMem+sideMem,
			cpuAlloc-dsCPU, appCPU+sideCPU, fitCPU, ruPlural(fitCPU, "под", "пода", "подов"), memAllocGi*1024-dsMem, appMem+sideMem, fitMem,
			bottleneck, fit, ruPlural(fit, "под", "пода", "подов"), tail),
	}, nil
}

// ciJobConditions — какие job GitHub Actions запустятся при заданном событии, статусе тестов, needs и if.
type ciJobConditions struct{}

func (ciJobConditions) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.ci_cd.job-conditions.v1", Spec: itembank.SpecDevOps, Topic: "ci_cd",
		Grade: itembank.GradeSenior, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.5, B: 0.8, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"ci", "github-actions"},
	}
}

type ciEvent struct {
	name, ref, desc string
}

// ciCond — условие if: выражение, содержит ли оно функцию статуса и его значение в контексте.
type ciCond struct {
	expr     string
	statusFn bool
	eval     func(ev ciEvent, ancestorFailed bool) bool
}

type ciJob struct {
	name  string
	needs []int
	cond  *ciCond
}

var (
	ciCondPush      = ciCond{"github.event_name == 'push'", false, func(ev ciEvent, _ bool) bool { return ev.name == "push" }}
	ciCondNotPR     = ciCond{"github.event_name != 'pull_request'", false, func(ev ciEvent, _ bool) bool { return ev.name != "pull_request" }}
	ciCondPR        = ciCond{"github.event_name == 'pull_request'", false, func(ev ciEvent, _ bool) bool { return ev.name == "pull_request" }}
	ciCondMain      = ciCond{"github.ref == 'refs/heads/main'", false, func(ev ciEvent, _ bool) bool { return ev.ref == "refs/heads/main" }}
	ciCondTag       = ciCond{"startsWith(github.ref, 'refs/tags/')", false, func(ev ciEvent, _ bool) bool { return strings.HasPrefix(ev.ref, "refs/tags/") }}
	ciCondMainPush  = ciCond{"github.event_name == 'push' && github.ref == 'refs/heads/main'", false, func(ev ciEvent, _ bool) bool { return ev.name == "push" && ev.ref == "refs/heads/main" }}
	ciCondAlways    = ciCond{"always()", true, func(ciEvent, bool) bool { return true }}
	ciCondFailure   = ciCond{"failure()", true, func(_ ciEvent, failed bool) bool { return failed }}
	ciCondAlwaysPsh = ciCond{"always() && github.event_name == 'push'", true, func(ev ciEvent, _ bool) bool { return ev.name == "push" }}
	ciCondAlwaysTag = ciCond{"always() && startsWith(github.ref, 'refs/tags/')", true, func(ev ciEvent, _ bool) bool { return strings.HasPrefix(ev.ref, "refs/tags/") }}
)

func ciAncestorFailed(jobs []ciJob, status []string, i int) bool {
	for _, d := range jobs[i].needs {
		if status[d] == "failure" || ciAncestorFailed(jobs, status, d) {
			return true
		}
	}
	return false
}

func ruJobStatus(s string) string {
	switch s {
	case "success":
		return "успешно"
	case "failure":
		return "упала"
	default:
		return "пропущена"
	}
}

// ciEvaluate вычисляет статус каждой job (success/failure/skipped) и объяснение по правилам GitHub Actions.
func ciEvaluate(jobs []ciJob, ev ciEvent, testFails bool) (status, reasons []string) {
	status = make([]string, len(jobs))
	reasons = make([]string, len(jobs))
	ctx := fmt.Sprintf("`github.event_name` = `%s`, `github.ref` = `%s`", ev.name, ev.ref)
	for i, j := range jobs {
		depsOK := true
		var bad []string
		for _, d := range j.needs {
			if status[d] != "success" {
				depsOK = false
				bad = append(bad, fmt.Sprintf("`%s` — %s", jobs[d].name, ruJobStatus(status[d])))
			}
		}
		failed := ciAncestorFailed(jobs, status, i)
		run := false
		switch {
		case j.cond == nil && len(j.needs) == 0:
			run = true
			reasons[i] = "нет ни `needs`, ни `if` → запущена"
		case j.cond == nil && depsOK:
			run = true
			reasons[i] = "все `needs` успешны, условия нет → запущена"
		case j.cond == nil:
			reasons[i] = fmt.Sprintf("`needs` не выполнены успешно (%s) → пропущена", strings.Join(bad, ", "))
		case j.cond.statusFn:
			run = j.cond.eval(ev, failed)
			detail := ctx
			if strings.Contains(j.cond.expr, "failure()") {
				detail = "ни одна предшествующая job не завершилась с ошибкой, а пропуск — не ошибка"
				if failed {
					detail = "предшествующая job завершилась с ошибкой"
				}
			}
			if run {
				reasons[i] = fmt.Sprintf("`%s` истинно (%s) и не требует успеха `needs` → запущена", j.cond.expr, detail)
			} else {
				reasons[i] = fmt.Sprintf("`%s` ложно (%s) → пропущена", j.cond.expr, detail)
			}
		case !depsOK:
			reasons[i] = fmt.Sprintf("`needs` не выполнены успешно (%s), а в `if` нет функций статуса — неявный `success()` ложен → пропущена", strings.Join(bad, ", "))
		case j.cond.eval(ev, failed):
			run = true
			reasons[i] = fmt.Sprintf("все `needs` успешны и `%s` истинно → запущена", j.cond.expr)
		default:
			reasons[i] = fmt.Sprintf("`%s` ложно (%s) → пропущена", j.cond.expr, ctx)
		}
		switch {
		case !run:
			status[i] = "skipped"
		case i == 1 && testFails:
			status[i] = "failure"
			reasons[i] = "запущена и завершилась с ошибкой"
		default:
			status[i] = "success"
		}
	}
	return status, reasons
}

func (f ciJobConditions) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F27)
	names := []string{
		itembank.Pick(r, []string{"lint", "static-check"}), itembank.Pick(r, []string{"test", "unit-tests"}),
		itembank.Pick(r, []string{"build", "docker-image"}), itembank.Pick(r, []string{"deploy-staging", "deploy-stage"}),
		itembank.Pick(r, []string{"deploy-prod", "release"}), itembank.Pick(r, []string{"notify", "notify-slack", "report"}),
	}
	feat := itembank.Pick(r, []string{"search", "billing-fix", "new-login", "metrics", "retry-queue"})
	pr := itembank.IntBetween(r, 100, 999)
	tag := fmt.Sprintf("v%d.%d.%d", 1+r.IntN(4), r.IntN(10), r.IntN(10))
	events := []ciEvent{
		{"push", "refs/heads/main", "событием `push` в ветку `main`"},
		{"push", "refs/heads/feature/" + feat, fmt.Sprintf("событием `push` в ветку `feature/%s`", feat)},
		{"pull_request", fmt.Sprintf("refs/pull/%d/merge", pr), fmt.Sprintf("событием `pull_request` (PR #%d в `main`)", pr)},
		{"push", "refs/tags/" + tag, fmt.Sprintf("событием `push` тега `%s`", tag)},
	}
	var jobs []ciJob
	var ev ciEvent
	var testFails bool
	var status, reasons []string
	for {
		ev = itembank.Pick(r, events)
		testFails = r.IntN(5) < 2
		jobs = []ciJob{
			{names[0], nil, nil},
			{names[1], []int{0}, nil},
			{names[2], []int{1}, itembank.Pick(r, []*ciCond{&ciCondPush, &ciCondNotPR, nil})},
			{names[3], []int{2}, itembank.Pick(r, []*ciCond{&ciCondMain, &ciCondPR, nil})},
			{names[4], itembank.Pick(r, [][]int{{2}, {3}}), itembank.Pick(r, []*ciCond{&ciCondTag, &ciCondMainPush, &ciCondAlwaysTag})},
			{names[5], itembank.Pick(r, [][]int{{1, 2}, {2}, {1}}), itembank.Pick(r, []*ciCond{&ciCondAlways, &ciCondFailure, &ciCondAlwaysPsh, nil})},
		}
		status, reasons = ciEvaluate(jobs, ev, testFails)
		ran := 0
		for _, s := range status[2:] {
			if s != "skipped" {
				ran++
			}
		}
		if ran >= 1 && ran <= 2 {
			break
		}
	}
	var sb strings.Builder
	sb.WriteString("jobs:\n")
	render := func(j ciJob) {
		fmt.Fprintf(&sb, "  %s:\n    runs-on: ubuntu-latest\n", j.name)
		if len(j.needs) > 0 {
			ns := make([]string, len(j.needs))
			for i, d := range j.needs {
				ns[i] = jobs[d].name
			}
			fmt.Fprintf(&sb, "    needs: [%s]\n", strings.Join(ns, ", "))
		}
		if j.cond != nil {
			fmt.Fprintf(&sb, "    if: %s\n", j.cond.expr)
		}
	}
	render(jobs[0])
	render(jobs[1])
	for _, i := range itembank.Shuffle(r, []int{2, 3, 4, 5}) {
		render(jobs[i])
	}
	outcome := "успешно"
	if testFails {
		outcome = "с ошибкой (упали тесты)"
	}
	rules := "Job запускается, если все job из её `needs` завершились успешно и условие `if` истинно; если в `if` нет функций статуса (`always()`, `failure()`), к нему неявно добавляется `success()`. " +
		"`always()` истинно всегда; `failure()` — когда хотя бы одна из предшествующих по `needs` job завершилась с ошибкой. Пропущенная job не считается успешной."
	askSkipped := r.IntN(2) == 0
	question := "Какие job будут запущены (а не пропущены)? Выберите все подходящие."
	if askSkipped {
		question = "Какие job будут пропущены (skipped)? Выберите все подходящие."
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Workflow GitHub Actions ниже (шаги job опущены) запущен %s; job `%s` завершилась %s. %s %s", ev.desc, names[1], outcome, rules, question),
		fmt.Sprintf("Ниже — workflow GitHub Actions без шагов. Его запуск вызван %s, job `%s` завершилась %s. %s %s", ev.desc, names[1], outcome, rules, question),
	})
	var correct, wrong, details []string
	for i, j := range jobs {
		opt := "`" + j.name + "`"
		if ran := status[i] != "skipped"; ran != askSkipped {
			correct = append(correct, opt)
		} else {
			wrong = append(wrong, opt)
		}
		details = append(details, opt+" — "+reasons[i])
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    stem,
		Code:    &itembank.CodeBlock{Lang: "yaml", Source: strings.TrimRight(sb.String(), "\n")},
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: strings.Join(details, "; ") + ". Типичные ошибки — считать, что условие без функций статуса запустит job после упавшей зависимости, " +
			"и что пропущенная зависимость «не мешает»: без `always()` она блокирует job так же, как упавшая.",
	}, nil
}

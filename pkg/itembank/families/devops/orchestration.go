package devops

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(portMapping{})
	itembank.Register(entrypointCmd{})
	itembank.Register(qosClass{})
	itembank.Register(hpaReplicas{})
	itembank.Register(probeRestart{})
}

// portMapping — публикация портов docker run -p: с хоста, из соседнего контейнера, внутри контейнера.
type portMapping struct{}

func (portMapping) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.docker.port-mapping.v1", Spec: itembank.SpecDevOps, Topic: "docker",
		Grade: itembank.GradeJunior, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.0, B: -0.9, C: 0.25}, TimeLimit: 60 * time.Second, Tags: []string{"docker"},
	}
}

func (f portMapping) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F05)
	name := itembank.Ident(r, "service")
	hostPorts := itembank.PickN(r, []int{8080, 8081, 8088, 8090, 9000, 9090, 3001, 5001, 7000, 18080}, 2)
	ctrPorts := itembank.PickN(r, []int{80, 3000, 8000, 5000, 4000, 8443, 9091, 6000}, 2)
	hp, hp2, cp, cp2 := hostPorts[0], hostPorts[1], ctrPorts[0], ctrPorts[1]
	cmd := fmt.Sprintf("docker run -d --name %s --network app-net \\\n  -p %d:%d -p %d:%d \\\n  registry.local/%s:%d.%d",
		name, hp, cp, hp2, cp2, name, 1+r.IntN(3), r.IntN(10))
	var stem, correct, explain string
	var distractors []string
	switch r.IntN(3) {
	case 0:
		stem = fmt.Sprintf("Контейнер запущен командой ниже; приложение внутри слушает HTTP на порту %d, метрики — на порту %d. "+
			"По какому адресу открыть HTTP-интерфейс в браузере на самой хост-машине?", cp, cp2)
		correct = fmt.Sprintf("http://localhost:%d", hp)
		distractors = []string{fmt.Sprintf("http://localhost:%d", cp), fmt.Sprintf("http://localhost:%d", hp2), fmt.Sprintf("http://localhost:%d", cp2)}
		explain = fmt.Sprintf("Формат `-p хост:контейнер`: порт %d контейнера опубликован на порту %d хоста, поэтому с хоста сервис доступен на localhost:%d. "+
			"Порт %d существует только в сетевом пространстве контейнера, а пара %d:%d относится к метрикам. Типичная ошибка — читать маппинг справа налево.",
			cp, hp, hp, cp, hp2, cp2)
	case 1:
		stem = fmt.Sprintf("Контейнер запущен командой ниже в пользовательской сети `app-net`; приложение слушает HTTP на порту %d. "+
			"Другой контейнер из той же сети обращается к нему по имени. Какой адрес нужно использовать?", cp)
		correct = fmt.Sprintf("http://%s:%d", name, cp)
		distractors = []string{fmt.Sprintf("http://%s:%d", name, hp), fmt.Sprintf("http://localhost:%d", hp), fmt.Sprintf("http://localhost:%d", cp)}
		explain = fmt.Sprintf("Внутри пользовательской сети Docker контейнеры обращаются друг к другу напрямую по имени и порту, на котором слушает процесс (%d); "+
			"публикация `-p` нужна только для доступа с хоста и извне, поэтому порт %d здесь не участвует. localhost внутри контейнера — это сам контейнер. "+
			"Типичная ошибка — использовать опубликованный порт хоста.", cp, hp)
	default:
		stem = fmt.Sprintf("Контейнер запущен командой ниже. На каком порту должен слушать процесс внутри контейнера, "+
			"чтобы он был доступен с хоста по адресу `localhost:%d`?", hp2)
		correct = strconv.Itoa(cp2)
		distractors = []string{strconv.Itoa(hp2), strconv.Itoa(hp), strconv.Itoa(cp)}
		explain = fmt.Sprintf("В `-p %d:%d` слева порт хоста, справа — порт контейнера: запросы на localhost:%d хоста попадают на порт %d внутри контейнера, "+
			"значит, процесс должен слушать именно %d. Типичная ошибка — считать, что процесс должен слушать опубликованный порт хоста.", hp2, cp2, hp2, cp2, cp2)
	}
	options, key := itembank.ShuffledOptions(r, correct, distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Code:        &itembank.CodeBlock{Lang: "bash", Source: cmd},
		Options:     options,
		Answer:      itembank.Answer{Keys: []string{key}},
		Explanation: explain,
	}, nil
}

// entrypointCmd — что выполнится при docker run с разными сочетаниями ENTRYPOINT и CMD.
type entrypointCmd struct{}

func (entrypointCmd) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.docker.entrypoint-cmd.v1", Spec: itembank.SpecDevOps, Topic: "docker",
		Grade: itembank.GradeMiddle, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.3, B: 0.3, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"docker"},
	}
}

var dockerWords = []string{"hello", "world", "start", "build", "deploy", "ready", "alpha", "beta", "gamma", "prod", "staging", "blue", "green", "worker", "api", "queue", "migrate", "serve"}

func jsonArray(xs []string) string {
	quoted := make([]string, len(xs))
	for i, x := range xs {
		quoted[i] = `"` + x + `"`
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func (f entrypointCmd) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F06)
	words := itembank.PickN(r, dockerWords, 6)
	ep := words[0 : 1+r.IntN(2)]
	cmd := words[2 : 3+r.IntN(2)]
	args := words[4 : 5+r.IntN(2)]
	image := fmt.Sprintf("%s:%d.%d", itembank.Ident(r, "service"), 1+r.IntN(3), r.IntN(10))
	epJSON := jsonArray(append([]string{"echo"}, ep...))
	join := func(xs ...[]string) string {
		var all []string
		for _, x := range xs {
			all = append(all, x...)
		}
		return strings.Join(all, " ")
	}
	var dockerfile, run, out, explain string
	switch r.IntN(6) {
	case 0:
		dockerfile = fmt.Sprintf("FROM alpine:3.20\nENTRYPOINT %s\nCMD %s", epJSON, jsonArray(cmd))
		run = "docker run --rm " + image
		out = join(ep, cmd)
		explain = fmt.Sprintf("При exec-форме обеих инструкций аргументы CMD дописываются к ENTRYPOINT: выполняется `echo %s`. "+
			"Типичная ошибка — считать, что CMD заменяет ENTRYPOINT или что CMD выполняется отдельной командой.", out)
	case 1:
		dockerfile = fmt.Sprintf("FROM alpine:3.20\nENTRYPOINT %s\nCMD %s", epJSON, jsonArray(cmd))
		run = "docker run --rm " + image + " " + join(args)
		out = join(ep, args)
		explain = fmt.Sprintf("Аргументы `docker run` после имени образа целиком заменяют CMD, а ENTRYPOINT остаётся: выполняется `echo %s`. "+
			"Типичная ошибка — дописывать аргументы к CMD (%s) вместо замены.", out, join(cmd))
	case 2:
		dockerfile = fmt.Sprintf("FROM alpine:3.20\nENTRYPOINT %s\nCMD %s", epJSON, jsonArray(cmd))
		run = "docker run --rm --entrypoint echo " + image + " " + join(args)
		out = join(args)
		explain = fmt.Sprintf("Флаг `--entrypoint` заменяет ENTRYPOINT образа и одновременно сбрасывает его CMD; аргументы после имени образа становятся "+
			"аргументами новой команды: выполняется `echo %s`. Типичная ошибка — оставлять слова из ENTRYPOINT или CMD образа в выводе.", out)
	case 3:
		dockerfile = fmt.Sprintf("FROM alpine:3.20\nENTRYPOINT echo %s\nCMD %s", join(ep), jsonArray(cmd))
		run = "docker run --rm " + image + " " + join(args)
		out = join(ep)
		explain = fmt.Sprintf("ENTRYPOINT в shell-форме запускается как `/bin/sh -c \"echo %s\"`, и к нему не дописываются ни CMD, ни аргументы "+
			"`docker run` — они просто игнорируются. Типичная ошибка — ожидать `echo %s`.", out, join(ep, args))
	case 4:
		dockerfile = fmt.Sprintf("FROM alpine:3.20\nCMD %s", jsonArray(append([]string{"echo"}, cmd...)))
		if r.IntN(2) == 0 {
			run = "docker run --rm " + image
			out = join(cmd)
			explain = fmt.Sprintf("Без ENTRYPOINT команда контейнера — это CMD целиком: `echo %s`. Типичная ошибка — считать, что без ENTRYPOINT "+
				"контейнер ничего не выполнит.", out)
		} else {
			run = "docker run --rm " + image + " echo " + join(args)
			out = join(args)
			explain = fmt.Sprintf("Аргументы `docker run` после имени образа полностью заменяют CMD, а ENTRYPOINT в образе нет, поэтому выполняется "+
				"ровно `echo %s`. Типичная ошибка — складывать CMD и аргументы (`echo %s %s`).", out, join(cmd), join(args))
		}
	default:
		dockerfile = fmt.Sprintf("FROM alpine:3.20\nENTRYPOINT %s\nCMD echo %s", epJSON, join(cmd))
		run = "docker run --rm " + image
		out = join(ep) + " /bin/sh -c echo " + join(cmd)
		explain = fmt.Sprintf("CMD в shell-форме превращается в `[\"/bin/sh\", \"-c\", \"echo %s\"]`, и этот массив дописывается к exec-ENTRYPOINT как "+
			"аргументы echo, поэтому в выводе буквально появляются `/bin/sh -c echo %s`. Типичная ошибка — ожидать `%s`: shell-форму CMD с exec-ENTRYPOINT "+
			"смешивать нельзя.", join(cmd), join(cmd), join(ep, cmd))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        fmt.Sprintf("Образ `%s` собран по Dockerfile ниже и запущен командой `%s`. Что будет выведено в stdout?", image, run),
		Code:        &itembank.CodeBlock{Lang: "dockerfile", Source: dockerfile},
		Answer:      itembank.Answer{Text: out},
		Explanation: explain,
	}, nil
}

// qosClass — какие поды получат заданный класс QoS по requests/limits контейнеров.
type qosClass struct{}

func (qosClass) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.kubernetes.qos-class.v1", Spec: itembank.SpecDevOps, Topic: "kubernetes",
		Grade: itembank.GradeMiddle, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.2, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"kubernetes"},
	}
}

type k8sContainer struct {
	name, reqCPU, reqMem, limCPU, limMem string
}

type k8sPod struct {
	name       string
	containers []k8sContainer
}

var (
	cpuValues = []string{"100m", "250m", "500m", "1"}
	memValues = []string{"128Mi", "256Mi", "512Mi", "1Gi"}
	podBases  = []string{"api", "worker", "scheduler", "gateway", "cache", "indexer", "notifier", "exporter", "cron", "proxy"}
)

// qosOf вычисляет класс QoS по правилам Kubernetes.
func qosOf(cs []k8sContainer) string {
	allEmpty, guaranteed := true, true
	for _, c := range cs {
		if c.reqCPU != "" || c.reqMem != "" || c.limCPU != "" || c.limMem != "" {
			allEmpty = false
		}
		if c.limCPU == "" || c.limMem == "" {
			guaranteed = false
		}
		if (c.reqCPU != "" && c.reqCPU != c.limCPU) || (c.reqMem != "" && c.reqMem != c.limMem) {
			guaranteed = false
		}
	}
	switch {
	case allEmpty:
		return "BestEffort"
	case guaranteed:
		return "Guaranteed"
	default:
		return "Burstable"
	}
}

func fmtResources(cpu, mem string) string {
	switch {
	case cpu == "" && mem == "":
		return "—"
	case cpu == "":
		return "memory " + mem
	case mem == "":
		return "cpu " + cpu
	default:
		return "cpu " + cpu + ", memory " + mem
	}
}

func (f qosClass) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F07)
	bases := itembank.PickN(r, podBases, 5)
	genContainer := func(name string) k8sContainer {
		ci, mi := r.IntN(3), r.IntN(3)
		c := k8sContainer{name: name}
		switch r.IntN(7) {
		case 0: // requests = limits
			c.reqCPU, c.reqMem, c.limCPU, c.limMem = cpuValues[ci], memValues[mi], cpuValues[ci], memValues[mi]
		case 1: // только limits — requests подставятся равными
			c.limCPU, c.limMem = cpuValues[ci], memValues[mi]
		case 2: // requests < limits
			c.reqCPU, c.reqMem, c.limCPU, c.limMem = cpuValues[ci], memValues[mi], cpuValues[ci+1], memValues[mi+1]
		case 3: // только requests
			c.reqCPU, c.reqMem = cpuValues[ci], memValues[mi]
		case 4: // ничего
		case 5: // только память, requests = limits
			c.reqMem, c.limMem = memValues[mi], memValues[mi]
		default: // только лимит CPU
			c.limCPU = cpuValues[ci]
		}
		return c
	}
	var pods []k8sPod
	var classes []string
	var target string
	for {
		pods = pods[:0]
		classes = classes[:0]
		counts := map[string]int{}
		for i := 0; i < 5; i++ {
			pod := k8sPod{name: fmt.Sprintf("%s-%04x", bases[i], r.IntN(0x10000))}
			pod.containers = append(pod.containers, genContainer("app"))
			if r.IntN(5) < 2 {
				pod.containers = append(pod.containers, genContainer(itembank.Pick(r, []string{"sidecar", "proxy", "log-agent"})))
			}
			cls := qosOf(pod.containers)
			counts[cls]++
			pods = append(pods, pod)
			classes = append(classes, cls)
		}
		var eligible []string
		for _, cls := range []string{"Guaranteed", "Burstable", "BestEffort"} {
			if counts[cls] >= 2 && counts[cls] <= 3 {
				eligible = append(eligible, cls)
			}
		}
		if len(eligible) > 0 {
			target = itembank.Pick(r, eligible)
			break
		}
	}
	var table strings.Builder
	table.WriteString("| Pod | Контейнер | requests | limits |\n|---|---|---|---|\n")
	var correct, wrong, reasons []string
	for i, p := range pods {
		for _, c := range p.containers {
			fmt.Fprintf(&table, "| `%s` | %s | %s | %s |\n", p.name, c.name, fmtResources(c.reqCPU, c.reqMem), fmtResources(c.limCPU, c.limMem))
		}
		opt := "`" + p.name + "`"
		if classes[i] == target {
			correct = append(correct, opt)
		} else {
			wrong = append(wrong, opt)
		}
		var why string
		switch classes[i] {
		case "Guaranteed":
			why = "у каждого контейнера заданы limits по cpu и memory, а requests равны им или опущены"
		case "BestEffort":
			why = "ни у одного контейнера нет ни requests, ни limits"
		default:
			why = "ресурсы заданы, но не у всех контейнеров оба лимита совпадают с requests"
		}
		reasons = append(reasons, fmt.Sprintf("`%s` — %s (%s)", p.name, classes[i], why))
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Для подов ниже указаны ресурсы контейнеров («—» — поле не задано). Какие поды получат класс QoS **%s**? "+
			"Выберите все подходящие.\n\n%s", target, table.String()),
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: "Guaranteed требует limits по cpu и memory у каждого контейнера и requests, равные limits (опущенные requests подставляются из limits); " +
			"BestEffort — полное отсутствие requests и limits во всех контейнерах; всё остальное — Burstable. " + strings.Join(reasons, "; ") + ". " +
			"Типичные ошибки — считать Guaranteed под с лимитом только по памяти или BestEffort под, где хотя бы один контейнер что-то запросил.",
	}, nil
}

// hpaReplicas — число реплик после цикла HPA с учётом допуска и границ min/max.
type hpaReplicas struct{}

func (hpaReplicas) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.kubernetes.hpa-replicas.v1", Spec: itembank.SpecDevOps, Topic: "kubernetes",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.8, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"kubernetes"},
	}
}

func (f hpaReplicas) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F08)
	app := itembank.Ident(r, "service")
	minR := itembank.IntBetween(r, 1, 3)
	maxR := itembank.IntBetween(r, 6, 20)
	cur := itembank.IntBetween(r, minR, maxR)
	target := itembank.Pick(r, []int{50, 60, 70, 75, 80})
	util := itembank.IntBetween(r, 15, 170)
	if r.IntN(4) == 0 {
		util = target + itembank.IntBetween(r, -6, 6) // около допуска
	}
	diff := util - target
	if diff < 0 {
		diff = -diff
	}
	within := 10*diff <= target // |util/target − 1| ≤ 0.1
	raw := float64(cur*util) / float64(target)
	desired := cur
	if !within {
		desired = (cur*util + target - 1) / target // ceil
	}
	clamped := desired
	switch {
	case clamped < minR:
		clamped = minR
	case clamped > maxR:
		clamped = maxR
	}
	manifest := fmt.Sprintf("apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nmetadata:\n  name: %s\nspec:\n  scaleTargetRef:\n"+
		"    apiVersion: apps/v1\n    kind: Deployment\n    name: %s\n  minReplicas: %d\n  maxReplicas: %d\n  metrics:\n  - type: Resource\n"+
		"    resource:\n      name: cpu\n      target:\n        type: Utilization\n        averageUtilization: %d", app, app, minR, maxR, target)
	var explain string
	switch {
	case within:
		explain = fmt.Sprintf("Отношение current/target = %d/%d = %.2f отличается от 1 не более чем на 10 %%, поэтому HPA ничего не меняет: остаётся %d реплик. "+
			"Типичная ошибка — пересчитывать реплики при любом отклонении от цели.", util, target, float64(util)/float64(target), cur)
	case clamped != desired:
		explain = fmt.Sprintf("desired = ceil(%d × %d / %d) = ceil(%.2f) = %d, но результат ограничен диапазоном [%d, %d], поэтому реплик станет %d. "+
			"Типичная ошибка — забыть про границы minReplicas/maxReplicas.", cur, util, target, raw, desired, minR, maxR, clamped)
	default:
		explain = fmt.Sprintf("desired = ceil(%d × %d / %d) = ceil(%.2f) = %d; отклонение %d/%d = %.2f выходит за допуск 10 %%, границы [%d, %d] не мешают. "+
			"Типичная ошибка — округлять вниз или делить target на current.", cur, util, target, raw, desired, util, target, float64(util)/float64(target), minR, maxR)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Сейчас у Deployment `%s` %d реплик, средняя утилизация CPU по подам — %d %% от requests. HPA считает "+
			"desiredReplicas = ceil(currentReplicas × currentUtilization / targetUtilization), не меняет число реплик, если отношение current/target "+
			"отличается от 1 не более чем на 10 %% (допуск по умолчанию), и ограничивает результат minReplicas/maxReplicas. "+
			"Сколько реплик будет после следующего цикла HPA?", app, cur, util),
		Code:        &itembank.CodeBlock{Lang: "yaml", Source: manifest},
		Answer:      itembank.Answer{Number: float64(clamped)},
		Explanation: explain,
	}, nil
}

// probeRestart — момент перезапуска контейнера (или вывода из Endpoints) по параметрам пробы.
type probeRestart struct{}

func (probeRestart) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.kubernetes.probe-restart.v1", Spec: itembank.SpecDevOps, Topic: "kubernetes",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.4, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"kubernetes"},
	}
}

func (f probeRestart) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F09)
	initial := itembank.Pick(r, []int{5, 10, 15, 20, 30})
	period := itembank.Pick(r, []int{5, 10, 15, 20})
	failures := itembank.IntBetween(r, 2, 5)
	path := itembank.Pick(r, []string{"/healthz", "/live", "/health", "/ready", "/status"})
	port := itembank.Pick(r, []int{8080, 8000, 3000, 9000})
	kind, effect := "livenessProbe", "kubelet перезапустит контейнер"
	if r.IntN(10) < 3 {
		kind, effect = "readinessProbe", "под будет помечен как не готовый и убран из Endpoints сервиса"
	}
	failFrom := 0
	if r.IntN(2) == 0 {
		failFrom = initial + r.IntN(4)*period + itembank.IntBetween(r, 1, period-1)
	}
	first := initial
	for first < failFrom {
		first += period
	}
	answer := first + (failures-1)*period
	manifest := fmt.Sprintf("%s:\n  httpGet:\n    path: %s\n    port: %d\n  initialDelaySeconds: %d\n  periodSeconds: %d\n"+
		"  timeoutSeconds: 1\n  successThreshold: 1\n  failureThreshold: %d", kind, path, port, initial, period, failures)
	var situation string
	if failFrom == 0 {
		situation = fmt.Sprintf("Контейнер стартовал в момент t = 0, но приложение с самого начала отвечает на `%s` кодом 500 (мгновенно, тайм-аут не срабатывает).", path)
	} else {
		situation = fmt.Sprintf("Контейнер стартовал в момент t = 0 и до t = %d с отвечал на `%s` кодом 200, а начиная с t = %d с отвечает кодом 500 "+
			"(мгновенно, тайм-аут не срабатывает).", failFrom, path, failFrom)
	}
	probes := make([]string, 0, 4)
	for i := 0; i < failures; i++ {
		probes = append(probes, strconv.Itoa(first+i*period))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   fmt.Sprintf("%s В какой момент t (в секундах от старта) %s?", situation, effect),
		Code:   &itembank.CodeBlock{Lang: "yaml", Source: manifest},
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Первая проба выполняется через initialDelaySeconds = %d с, далее каждые periodSeconds = %d с. Первая неудачная проба — t = %d; "+
			"решение принимается после failureThreshold = %d неудач подряд, то есть на пробах t = %s. Итог: %d с. "+
			"Типичные ошибки — умножать failureThreshold на период без вычитания единицы или отсчитывать от момента сбоя, а не от ближайшей пробы.",
			initial, period, first, failures, strings.Join(probes, ", "), answer),
	}, nil
}

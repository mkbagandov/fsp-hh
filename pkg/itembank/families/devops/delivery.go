package devops

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(stageOrder{})
	itembank.Register(cacheKey{})
	itembank.Register(canaryTraffic{})
	itembank.Register(terraformPlan{})
	itembank.Register(ansibleIdempotency{})
	itembank.Register(incidentTriage{})
}

// stageOrder — порядок выполнения job по зависимостям needs.
type stageOrder struct{}

func (stageOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.ci_cd.stage-order.v1", Spec: itembank.SpecDevOps, Topic: "ci_cd",
		Grade: itembank.GradeJunior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.0, B: -0.5, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"ci", "github-actions"},
	}
}

var (
	ciChain    = [][]string{{"lint", "static-check"}, {"build", "compile"}, {"unit-tests", "test"}, {"docker-image", "package"}, {"deploy-staging", "deploy-stage"}, {"e2e-tests", "smoke-tests"}, {"deploy-prod", "release"}}
	ciFloaters = []string{"security-scan", "sbom", "notify-slack", "license-check", "sonar-analysis"}
)

func (f stageOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F0A)
	idx := itembank.PickN(r, []int{0, 1, 2, 3, 4, 5, 6}, 4)
	sort.Ints(idx)
	chain := make([]string, 0, 5)
	for _, i := range idx {
		chain = append(chain, itembank.Pick(r, ciChain[i]))
	}
	// Job без устоявшегося места в конвейере: его позицию можно узнать только из needs.
	pos := r.IntN(5)
	chain = append(chain[:pos], append([]string{itembank.Pick(r, ciFloaters)}, chain[pos:]...)...)
	var sb strings.Builder
	sb.WriteString("jobs:\n")
	for _, i := range r.Perm(len(chain)) {
		fmt.Fprintf(&sb, "  %s:\n    runs-on: ubuntu-latest\n", chain[i])
		if i > 0 {
			fmt.Fprintf(&sb, "    needs: [%s]\n", chain[i-1])
		}
	}
	options, order := itembank.OrderingOptions(r, chain)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: "Ниже — фрагмент workflow GitHub Actions (шаги job опущены). Расставьте job в порядке их выполнения: " +
			"каждая job стартует только после успешного завершения всех job из своего `needs`.",
		Code:    &itembank.CodeBlock{Lang: "yaml", Source: sb.String()},
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: "Граф зависимостей линейный: " + strings.Join(chain, " → ") + ". Job без `needs` стартует первой, а `needs` указывает " +
			"на предшественника, а не на последователя. Типичная ошибка — выстраивать порядок по смыслу имён или по порядку объявления в файле, а не по `needs`.",
	}, nil
}

// cacheKey — поведение actions/cache при изменении lock-файла и наличии restore-keys.
type cacheKey struct{}

func (cacheKey) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.ci_cd.cache-key.v1", Spec: itembank.SpecDevOps, Topic: "ci_cd",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.1, C: 0.25}, TimeLimit: 120 * time.Second, Tags: []string{"ci", "github-actions"},
	}
}

type cacheEco struct {
	prefix, path, lock, install string
	srcFiles                    []string
}

var cacheEcos = []cacheEco{
	{"npm", "~/.npm", "package-lock.json", "npm ci", []string{"src/app.ts", "src/components/Header.tsx", "README.md"}},
	{"pip", "~/.cache/pip", "requirements.txt", "pip install -r requirements.txt", []string{"app/views.py", "tests/test_api.py", "README.md"}},
	{"go", "~/go/pkg/mod", "go.sum", "go mod download", []string{"internal/handler.go", "cmd/api/main.go", "docs/api.md"}},
	{"maven", "~/.m2/repository", "pom.xml", "mvn -B -q dependency:go-offline", []string{"src/main/java/App.java", "src/test/java/AppTest.java", "README.md"}},
}

func (f cacheKey) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F0B)
	e := itembank.Pick(r, cacheEcos)
	osName := itembank.Pick(r, []string{"Linux", "Windows", "macOS"})
	hash := fmt.Sprintf("%08x", r.Uint32())
	withRestore := r.IntN(2) == 0
	lockChanged := r.IntN(2) == 0
	changed := e.lock
	if !lockChanged {
		changed = itembank.Pick(r, e.srcFiles)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "- uses: actions/cache@v4\n  with:\n    path: %s\n    key: %s-${{ runner.os }}-${{ hashFiles('%s') }}\n", e.path, e.prefix, e.lock)
	if withRestore {
		fmt.Fprintf(&sb, "    restore-keys: |\n      %s-${{ runner.os }}-\n", e.prefix)
	}
	fmt.Fprintf(&sb, "- run: %s", e.install)
	exactHit := "Точное попадание по ключу: зависимости восстановлены из кэша, новый кэш после job не сохраняется"
	partialHit := "Промах по точному ключу; по `restore-keys` восстановится кэш предыдущего запуска, установка докачает только разницу, после job сохранится кэш с новым ключом"
	fullMiss := "Полный промах: кэш не восстанавливается, зависимости скачиваются с нуля, после job сохранится кэш с новым ключом"
	alwaysWrong := "Точное попадание по ключу, но после job кэш будет перезаписан, так как в коммите изменились файлы репозитория"
	var correct, explain string
	switch {
	case !lockChanged:
		correct = exactHit
		explain = fmt.Sprintf("`hashFiles('%s')` считает хеш только от файла зависимостей, изменения в `%s` на ключ не влияют — ключ совпал с сохранённым, "+
			"это точное попадание. При точном попадании actions/cache не пересохраняет кэш. Типичная ошибка — считать, что любой коммит меняет ключ.", e.lock, changed)
	case withRestore:
		correct = partialHit
		explain = fmt.Sprintf("Хеш изменившегося `%s` дал новый ключ, которого в хранилище нет; `restore-keys` ищет по префиксу `%s-%s-` и восстанавливает "+
			"ближайший кэш прошлого запуска, поэтому большая часть зависимостей уже на месте; после успешного job кэш сохраняется под новым ключом. "+
			"Типичная ошибка — ожидать точного попадания или полного промаха.", e.lock, e.prefix, osName)
	default:
		correct = fullMiss
		explain = fmt.Sprintf("Хеш изменившегося `%s` дал новый ключ, а `restore-keys` не задан — восстанавливать нечего, `%s` скачивает всё заново; "+
			"после job кэш сохраняется под новым ключом. Типичная ошибка — считать, что кэш восстановится по частичному совпадению без restore-keys.", e.lock, e.install)
	}
	options, key := itembank.ShuffledOptions(r, correct, exactHit, partialHit, fullMiss, alwaysWrong)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("В workflow GitHub Actions есть шаг кэширования (ниже). Предыдущий запуск в этой ветке сохранил кэш с ключом `%s-%s-%s`. "+
			"В новом коммите изменён только файл `%s`. Что произойдёт на шаге кэша?", e.prefix, osName, hash, changed),
		Code:        &itembank.CodeBlock{Lang: "yaml", Source: sb.String()},
		Options:     options,
		Answer:      itembank.Answer{Keys: []string{key}},
		Explanation: explain,
	}, nil
}

// canaryTraffic — доля трафика и нагрузка на поды при canary-выкатке.
type canaryTraffic struct{}

func (canaryTraffic) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.ci_cd.canary-traffic.v1", Spec: itembank.SpecDevOps, Topic: "ci_cd",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.6, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"kubernetes", "deploy"},
	}
}

func (f canaryTraffic) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F0C)
	app := itembank.Ident(r, "service")
	var stem, explain string
	var answer, tol float64
	switch r.IntN(3) {
	case 0:
		stable := itembank.IntBetween(r, 4, 12)
		canary := itembank.IntBetween(r, 1, 3)
		answer, tol = float64(canary)/float64(stable+canary)*100, 0.1
		stem = fmt.Sprintf("Service `%s` балансирует запросы равномерно по всем подам с селектором `app=%s`. В стабильном Deployment %d подов, "+
			"в canary-Deployment — %d. Какая доля трафика (в процентах) попадёт на canary? Ответ с точностью до десятых.", app, app, stable, canary)
		explain = fmt.Sprintf("Service не различает Deployment'ы: трафик делится поровну между всеми %d + %d = %d подами, canary получает %d/%d = %.1f %%. "+
			"Типичная ошибка — делить на число стабильных подов (%d/%d = %.1f %%).", stable, canary, stable+canary, canary, stable+canary, answer,
			canary, stable, float64(canary)/float64(stable)*100)
	case 1:
		stable := itembank.IntBetween(r, 5, 15)
		pct := itembank.Pick(r, []int{5, 10, 15, 20, 25})
		need := (pct*stable + (100 - pct) - 1) / (100 - pct) // ceil(pct·stable / (100 − pct))
		answer, tol = float64(need), 0
		stem = fmt.Sprintf("Service `%s` распределяет запросы равномерно по подам стабильного Deployment (%d подов) и canary-Deployment. "+
			"Сколько минимум canary-подов нужно запустить, чтобы на canary приходилось не менее %d %% трафика?", app, stable, pct)
		explain = fmt.Sprintf("Доля canary = c / (%d + c) ≥ %d %%, откуда c ≥ %d × %d / (100 − %d) = %.2f; минимальное целое — %d. "+
			"Типичная ошибка — брать %d %% от %d (%.1f) и забывать, что canary-поды тоже входят в знаменатель.",
			stable, pct, pct, stable, pct, float64(pct*stable)/float64(100-pct), need, pct, stable, float64(pct*stable)/100)
	default:
		stable := itembank.IntBetween(r, 4, 10)
		canary := itembank.IntBetween(r, 1, 2)
		weight := itembank.Pick(r, []int{5, 10, 20, 25, 30})
		rps := itembank.Pick(r, []int{600, 900, 1200, 1800, 2400, 3000})
		canaryTotal := float64(rps*weight) / 100
		answer, tol = canaryTotal/float64(canary), 0.1
		stablePer := float64(rps*(100-weight)) / 100 / float64(stable)
		stem = fmt.Sprintf("Ingress направляет %d %% запросов на canary-версию `%s`, остальное — на стабильную. Всего поступает %d запросов/с; "+
			"у стабильной версии %d подов, у canary — %d. Сколько запросов в секунду получает один canary-под? Ответ с точностью до десятых.",
			weight, app, rps, stable, canary)
		explain = fmt.Sprintf("На canary уходит %d × %d %% = %.0f запросов/с, они делятся между %d подами: %.1f на под. Для сравнения стабильный под "+
			"получает %.1f запросов/с — canary-поды при весовой маршрутизации легко перегрузить. Типичная ошибка — делить весь трафик на общее число подов, "+
			"забывая про веса.", rps, weight, canaryTotal, canary, answer, stablePer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Number: answer, Tolerance: tol},
		Explanation: explain,
	}, nil
}

// terraformPlan — сколько ресурсов попадёт в add/change/destroy по различиям state и конфигурации.
type terraformPlan struct{}

func (terraformPlan) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.iac.terraform-plan.v1", Spec: itembank.SpecDevOps, Topic: "iac",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.8, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"terraform"},
	}
}

type tfAttr struct {
	name     string
	forceNew bool
	vals     [2]string
}

type tfResource struct {
	addr  string
	attrs []tfAttr
}

var tfResources = []tfResource{
	{"aws_instance.web", []tfAttr{{"ami", true, [2]string{"ami-0a1b2c3d", "ami-0f9e8d7c"}}, {"instance_type", false, [2]string{"t3.small", "t3.medium"}}, {"tags.env", false, [2]string{"staging", "prod"}}}},
	{"aws_s3_bucket.assets", []tfAttr{{"bucket", true, [2]string{"acme-assets-v1", "acme-assets-v2"}}, {"tags.owner", false, [2]string{"platform", "frontend"}}}},
	{"aws_security_group.api", []tfAttr{{"name", true, [2]string{"api-sg", "api-sg-v2"}}, {"description", true, [2]string{"API ingress", "API ingress rules"}}, {"tags.team", false, [2]string{"core", "payments"}}}},
	{"aws_ebs_volume.data", []tfAttr{{"availability_zone", true, [2]string{"eu-central-1a", "eu-central-1b"}}, {"size", false, [2]string{"100", "200"}}, {"type", false, [2]string{"gp2", "gp3"}}}},
	{"aws_sqs_queue.jobs", []tfAttr{{"name", true, [2]string{"jobs-main", "jobs-primary"}}, {"visibility_timeout_seconds", false, [2]string{"30", "60"}}}},
	{"aws_iam_role.deploy", []tfAttr{{"name", true, [2]string{"deploy-role", "deployer"}}, {"description", false, [2]string{"CI deploy role", "Deploy role for CI"}}}},
	{"aws_db_instance.primary", []tfAttr{{"availability_zone", true, [2]string{"eu-central-1a", "eu-central-1c"}}, {"engine_version", false, [2]string{"15.4", "16.1"}}, {"instance_class", false, [2]string{"db.t3.medium", "db.t3.large"}}}},
}

func pickAttr(r *rand.Rand, attrs []tfAttr, forceNew bool) tfAttr {
	var pool []tfAttr
	for _, a := range attrs {
		if a.forceNew == forceNew {
			pool = append(pool, a)
		}
	}
	return itembank.Pick(r, pool)
}

func (f terraformPlan) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F12)
	resources := itembank.PickN(r, tfResources, itembank.IntBetween(r, 4, 5))
	var table strings.Builder
	table.WriteString("| Ресурс | Отличие конфигурации от state |\n|---|---|\n")
	adds, changes, destroys, replaces := 0, 0, 0, 0
	var details []string
	for _, res := range resources {
		var diff, verdict string
		switch itembank.Pick(r, []int{0, 1, 1, 2, 2, 3, 4}) {
		case 0:
			diff, verdict = "без изменений", "не затрагивается"
		case 1:
			a := pickAttr(r, res.attrs, false)
			diff = fmt.Sprintf("`%s`: %s → %s", a.name, a.vals[0], a.vals[1])
			changes++
			verdict = "обновление на месте (change)"
		case 2:
			a := pickAttr(r, res.attrs, true)
			diff = fmt.Sprintf("`%s`: %s → %s (forces replacement)", a.name, a.vals[0], a.vals[1])
			if r.IntN(2) == 0 {
				b := pickAttr(r, res.attrs, false)
				diff += fmt.Sprintf("; `%s`: %s → %s", b.name, b.vals[0], b.vals[1])
			}
			replaces++
			verdict = "пересоздание: destroy + create (по единице в add и в destroy, в change не входит)"
		case 3:
			diff, verdict = "удалён из конфигурации (в state есть)", "destroy"
			destroys++
		default:
			diff, verdict = "добавлен в конфигурацию (в state нет)", "add"
			adds++
		}
		fmt.Fprintf(&table, "| `%s` | %s |\n", res.addr, diff)
		details = append(details, fmt.Sprintf("`%s` — %s", res.addr, verdict))
	}
	totals := map[string]int{"add": adds + replaces, "change": changes, "destroy": destroys + replaces}
	asked := itembank.Pick(r, []string{"add", "change", "destroy"})
	line := "Plan: _ to add, _ to change, _ to destroy"
	line = strings.Replace(line, "_ to "+asked, "N to "+asked, 1)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Ниже — отличия конфигурации Terraform от текущего state; атрибуты с пометкой `forces replacement` провайдер не умеет менять "+
			"на месте. Какое число окажется на месте **N** в итоговой строке `%s`?\n\n%s", line, table.String()),
		Answer: itembank.Answer{Number: float64(totals[asked])},
		Explanation: fmt.Sprintf("%s. Итого: add = новых %d + пересоздаваемых %d = %d; change = %d; destroy = удалённых %d + пересоздаваемых %d = %d. "+
			"Типичная ошибка — засчитывать пересоздание как change или учитывать его только в одной из колонок add/destroy.",
			strings.Join(details, "; "), adds, replaces, adds+replaces, changes, destroys, replaces, destroys+replaces),
	}, nil
}

// ansibleIdempotency — сколько задач отчитаются changed при повторном запуске playbook.
type ansibleIdempotency struct{}

func (ansibleIdempotency) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.iac.ansible-idempotency.v1", Spec: itembank.SpecDevOps, Topic: "iac",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.3, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"ansible"},
	}
}

type ansTask struct {
	yaml    string
	changed bool
	why     string
}

func (f ansibleIdempotency) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F13)
	pkg := itembank.Pick(r, []string{"nginx", "redis-server", "haproxy", "postgresql-16", "chrony"})
	dir := itembank.Pick(r, []string{"/opt/app", "/srv/api", "/opt/worker", "/srv/portal"})
	svc := itembank.Ident(r, "service")
	user := itembank.Pick(r, []string{"deploy", "app", "svc", "runner"})
	idempotent := []ansTask{
		{fmt.Sprintf("- name: Install %s\n  apt: { name: %s, state: present }", pkg, pkg), false, ""},
		{fmt.Sprintf("- name: Create app dir\n  file: { path: %s, state: directory, mode: \"0755\" }", dir), false, ""},
		{fmt.Sprintf("- name: Render config\n  template: { src: %s.conf.j2, dest: /etc/%s/%s.conf }", svc, svc, svc), false, ""},
		{fmt.Sprintf("- name: Copy unit file\n  copy: { src: %s.service, dest: /etc/systemd/system/%s.service }", svc, svc), false, ""},
		{fmt.Sprintf("- name: Enable service\n  service: { name: %s, state: started, enabled: true }", svc), false, ""},
		{fmt.Sprintf("- name: Create user\n  user: { name: %s, state: present }", user), false, ""},
		{"- name: Tune swappiness\n  lineinfile: { path: /etc/sysctl.conf, regexp: '^vm.swappiness', line: 'vm.swappiness = 10' }", false, ""},
		{fmt.Sprintf("- name: Init database\n  command: %s/bin/init-db\n  args: { creates: %s/.initialized }", dir, dir), false, ""},
		{fmt.Sprintf("- name: Check version\n  shell: %s/bin/app --version\n  changed_when: false", dir), false, ""},
	}
	nonIdempotent := []ansTask{
		{fmt.Sprintf("- name: Build app\n  command: make -C %s install", dir), true, "`command` без `creates`/`removes`/`changed_when` всегда отчитывается changed"},
		{fmt.Sprintf("- name: Add motd line\n  shell: echo \"deployed %s\" >> /etc/motd", svc), true, "`shell` всегда changed, а строка к тому же дописывается при каждом запуске"},
		{"- name: Reload systemd\n  command: systemctl daemon-reload", true, "`command` без условий — changed при каждом запуске"},
		{fmt.Sprintf("- name: Download installer\n  shell: curl -sSL https://get.example.com/%s.sh | sh", svc), true, "`shell` без `creates`/`changed_when` — всегда changed"},
		{fmt.Sprintf("- name: Restart service\n  service: { name: %s, state: restarted }", svc), true, "`state: restarted` перезапускает сервис при каждом запуске — всегда changed"},
	}
	k := itembank.IntBetween(r, 1, 3)
	tasks := append(itembank.PickN(r, nonIdempotent, k), itembank.PickN(r, idempotent, 6-k)...)
	tasks = itembank.Shuffle(r, tasks)
	count := 0
	var yaml, culprits []string
	for _, t := range tasks {
		yaml = append(yaml, t.yaml)
		if t.changed {
			count++
			name := strings.TrimPrefix(strings.SplitN(t.yaml, "\n", 2)[0], "- name: ")
			culprits = append(culprits, fmt.Sprintf("«%s» — %s", name, t.why))
		}
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: "Playbook ниже уже один раз успешно выполнен на хосте. Его запускают повторно на том же хосте; вручную там ничего не меняли. " +
			"Сколько задач отчитаются `changed`?",
		Code:   &itembank.CodeBlock{Lang: "yaml", Source: strings.Join(yaml, "\n")},
		Answer: itembank.Answer{Number: float64(count)},
		Explanation: fmt.Sprintf("Changed при повторном запуске: %s. Остальные модули декларативны: они сравнивают желаемое состояние с фактическим и "+
			"отчитываются ok; `command` с `creates` пропускается, когда файл уже есть, а `changed_when: false` подавляет changed. "+
			"Типичные ошибки — считать `command`/`shell` идемпотентными или не замечать `state: restarted`.", strings.Join(culprits, "; ")),
	}, nil
}

// incidentTriage — порядок действий дежурного при инциденте.
type incidentTriage struct{}

func (incidentTriage) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.incident_response.triage-order.v1", Spec: itembank.SpecDevOps, Topic: "incident_response",
		Grade: itembank.GradeMiddle, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.1, B: 0.1, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"sre", "incident"},
	}
}

func clock(r *rand.Rand) time.Time {
	return time.Date(2026, 1, 1, itembank.IntBetween(r, 8, 20), itembank.IntBetween(r, 0, 59), 0, 0, time.UTC)
}

func (f incidentTriage) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F14)
	svc := itembank.Ident(r, "service")
	t0 := clock(r)
	var summary string
	var steps []string
	switch r.IntN(4) {
	case 0:
		major, minor, patch := 1+r.IntN(4), r.IntN(10), 1+r.IntN(9)
		pct, thr := itembank.IntBetween(r, 12, 45), itembank.Pick(r, []int{1, 2, 5})
		t1 := t0.Add(time.Duration(itembank.IntBetween(r, 2, 9)) * time.Minute)
		summary = fmt.Sprintf("в %s сработал алерт `HighErrorRate` по сервису `%s`: доля ответов 5xx выросла до %d %%", t1.Format("15:04"), svc, pct)
		steps = []string{
			fmt.Sprintf("Подтвердить алерт, взять роль ведущего инцидента и открыть канал `#inc-%s`", svc),
			fmt.Sprintf("Сопоставить начало роста 5xx (%s) с выкаткой `%s` версии v%d.%d.%d в %s и выдвинуть гипотезу о причине", t1.Format("15:04"), svc, major, minor, patch, t0.Format("15:04")),
			fmt.Sprintf("Откатить `%s` на предыдущую версию v%d.%d.%d", svc, major, minor, patch-1),
			fmt.Sprintf("Убедиться по дашборду, что доля 5xx вернулась ниже %d %%, и объявить инцидент закрытым", thr),
			"Провести постмортем без поиска виноватых и завести задачи по предотвращению повтора",
		}
	case 1:
		host := fmt.Sprintf("db-%02d.prod", itembank.IntBetween(r, 1, 12))
		pct, pct2, warn, days := itembank.IntBetween(r, 94, 99), itembank.IntBetween(r, 60, 75), itembank.IntBetween(r, 78, 85), itembank.Pick(r, []int{7, 14, 30})
		summary = fmt.Sprintf("на `%s` сработал алерт `DiskAlmostFull`: раздел `/var` заполнен на %d %%, PostgreSQL не может писать WAL", host, pct)
		steps = []string{
			fmt.Sprintf("Подтвердить алерт и оценить влияние: какие сервисы зависят от БД на `%s`", host),
			fmt.Sprintf("Найти, что занимает место: `du -sh /var/log/* /var/lib/postgresql/*` на `%s`", host),
			fmt.Sprintf("Освободить место безопасно: сжать и удалить ротированные логи старше %d дней, не трогая каталог WAL", days),
			fmt.Sprintf("Проверить, что запись в БД восстановилась и заполнение раздела опустилось ниже %d %%", pct2),
			fmt.Sprintf("Оформить постмортем: настроить ротацию логов и алерт на %d %% заполнения", warn),
		}
	case 2:
		host := fmt.Sprintf("%s.%s.example.com", itembank.Pick(r, []string{"api", "auth", "pay", "www"}), svc)
		days := itembank.Pick(r, []int{14, 21, 30})
		summary = fmt.Sprintf("клиенты `%s` получают ошибку сертификата при подключении к `%s`", svc, host)
		steps = []string{
			"Подтвердить алерт и оценить охват: какие клиенты и интеграции затронуты",
			fmt.Sprintf("Проверить сертификат: `openssl s_client -connect %s:443` показывает, что срок истёк в %s", host, t0.Format("15:04")),
			fmt.Sprintf("Перевыпустить сертификат для `%s`", host),
			"Установить новый сертификат на балансировщик и проверить цепочку доверия",
			fmt.Sprintf("Настроить автопродление и алерт за %d дней до истечения, написать постмортем", days),
		}
	default:
		lim := itembank.Pick(r, []int{256, 512, 768, 1024})
		summary = fmt.Sprintf("в %s сработал алерт `PodRestarting`: поды `%s` перезапускаются каждые несколько минут", t0.Format("15:04"), svc)
		steps = []string{
			"Подтвердить алерт и оценить влияние на пользователей: ошибки, задержки",
			fmt.Sprintf("Увидеть в `kubectl describe pod` причину `OOMKilled` при лимите памяти %d Mi", lim),
			fmt.Sprintf("Снять симптом: поднять лимит памяти `%s` до %d Mi и раскатить изменение", svc, lim*2),
			"Убедиться, что перезапуски прекратились и ошибки у пользователей ушли",
			"Снять профиль памяти, найти утечку и оформить постмортем с задачей на исправление",
		}
	}
	options, order := itembank.OrderingOptions(r, steps)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    fmt.Sprintf("Инцидент: %s. Расставьте действия дежурного инженера в правильном порядке.", summary),
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: "Порядок реагирования: подтвердить алерт и оценить влияние → локализовать вероятную причину → устранить или смягчить → " +
			"проверить восстановление по метрикам → закрыть инцидент постмортемом. Устранять нельзя раньше, чем понятно что; проверять — раньше, чем " +
			"что-то изменили. Типичные ошибки — начинать с глубокого поиска первопричины вместо быстрого смягчения или откладывать постмортем «до лучших времён».",
	}, nil
}

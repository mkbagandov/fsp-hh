package devops

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(chmodUmask{})
	itembank.Register(pipelineOutput{})
	itembank.Register(signalExitCode{})
	itembank.Register(cronNextRun{})
}

// chmodUmask — права доступа в восьмеричной записи: по строке ls -l, после umask
// или после chmod с символьными операциями.
type chmodUmask struct{}

func (chmodUmask) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.linux_shell.chmod-umask.v1", Spec: itembank.SpecDevOps, Topic: "linux_shell",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.0, B: -0.8, C: 0}, TimeLimit: 60 * time.Second, Tags: []string{"linux", "bash"},
	}
}

var (
	permFiles = []string{"deploy.sh", "app.conf", "backup.sh", "notes.txt", "run.sh", "config.yaml", "service.env", "report.csv", "healthcheck.sh", "nginx.conf", "release.log", "authorized_keys"}
	permDirs  = []string{"releases", "backups", "logs", "uploads", "cache", "build"}
	permUsers = []string{"deploy", "app", "www-data", "ci", "postgres", "ops"}
	umasks    = []struct {
		text string
		bits int
	}{{"022", 0o022}, {"027", 0o027}, {"002", 0o002}, {"077", 0o077}, {"007", 0o007}, {"037", 0o037}, {"026", 0o026}, {"066", 0o066}}
	chmodSpecs = []string{"u+x", "g+w", "o-r", "g-w", "o=r", "ug+x", "go-w", "a+r", "u=rw", "g=r", "o+x", "a-x", "o=", "g-x"}
)

// permString возвращает rwx-строку для 9-битной маски прав.
func permString(mode int) string {
	const letters = "rwxrwxrwx"
	var sb strings.Builder
	for i := 0; i < 9; i++ {
		if mode&(1<<(8-i)) != 0 {
			sb.WriteByte(letters[i])
		} else {
			sb.WriteByte('-')
		}
	}
	return sb.String()
}

// permOctal возвращает права как трёхзначное число из восьмеричных цифр (0o644 → 644).
func permOctal(mode int) int {
	return (mode>>6)*100 + ((mode>>3)&7)*10 + mode&7
}

// permDigits расписывает каждую триаду: «владелец rw- = 6».
func permDigits(mode int) string {
	who := []string{"владелец", "группа", "остальные"}
	s := permString(mode)
	parts := make([]string, 3)
	for i := 0; i < 3; i++ {
		parts[i] = fmt.Sprintf("%s %s = %d", who[i], s[3*i:3*i+3], (mode>>(6-3*i))&7)
	}
	return strings.Join(parts, ", ")
}

// applyChmod применяет одну символьную операцию chmod вида u+x, go-w, o=r.
func applyChmod(mode int, spec string) int {
	i := strings.IndexAny(spec, "+-=")
	who, op, perms := spec[:i], spec[i], spec[i+1:]
	bits := 0
	for _, p := range perms {
		switch p {
		case 'r':
			bits |= 4
		case 'w':
			bits |= 2
		case 'x':
			bits |= 1
		}
	}
	for _, w := range who {
		var shifts []int
		switch w {
		case 'u':
			shifts = []int{6}
		case 'g':
			shifts = []int{3}
		case 'o':
			shifts = []int{0}
		case 'a':
			shifts = []int{6, 3, 0}
		}
		for _, s := range shifts {
			switch op {
			case '+':
				mode |= bits << s
			case '-':
				mode &^= bits << s
			case '=':
				mode = mode&^(7<<s) | bits<<s
			}
		}
	}
	return mode
}

func (f chmodUmask) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F01)
	file := itembank.Pick(r, permFiles)
	user := itembank.Pick(r, permUsers)
	var stem, explain string
	var mode int
	switch r.IntN(3) {
	case 0:
		mode = 0o400 | r.IntN(0o400) // владелец хотя бы читает — первая цифра не ноль
		stem = fmt.Sprintf("Команда `ls -l` показала строку:\n\n`-%s 1 %s %s %d Mar %2d 10:%02d %s`\n\n"+
			"Какое восьмеричное число нужно передать `chmod`, чтобы выставить точно такие же права другому файлу? Ответ — число, например 644.",
			permString(mode), user, user, itembank.IntBetween(r, 120, 9800), itembank.IntBetween(r, 1, 28), itembank.IntBetween(r, 0, 59), file)
		explain = fmt.Sprintf("Каждая триада переводится в цифру как r = 4, w = 2, x = 1: %s. Итого %d. "+
			"Типичная ошибка — перепутать порядок триад (владелец, группа, остальные) или посчитать x как 2.", permDigits(mode), permOctal(mode))
	case 1:
		um := itembank.Pick(r, umasks)
		base, kind, cmd := 0o666, "файл", "touch "+file
		if r.IntN(2) == 0 {
			base, kind, cmd = 0o777, "каталог", "mkdir "+itembank.Pick(r, permDirs)
		}
		mode = base &^ um.bits
		stem = fmt.Sprintf("В сессии пользователя `%s` установлен `umask %s`. Какие права в восьмеричной записи получит новый %s, "+
			"созданный командой `%s`? Ответ — число, например 644.", user, um.text, kind, cmd)
		explain = fmt.Sprintf("umask снимает указанные биты с базовых прав: для нового %sа база %03o, %03o без битов %s даёт %03o (%s). "+
			"Типичная ошибка — вычитать umask арифметически или брать базу 777 для файла: без явного chmod файл не получает бит x.",
			kind, base, base, um.text, mode, permDigits(mode))
	default:
		start := itembank.Pick(r, []int{0o644, 0o640, 0o600, 0o755, 0o750, 0o664, 0o700, 0o444, 0o660})
		specs := itembank.PickN(r, chmodSpecs, itembank.IntBetween(r, 2, 3))
		mode = start
		for _, s := range specs {
			mode = applyChmod(mode, s)
		}
		stem = fmt.Sprintf("Файл `%s` имеет права `%s` (%03o). Выполнили `chmod %s %s`. Какие права в восьмеричной записи будут у файла "+
			"после команды? Ответ — число, например 644.", file, permString(start), start, strings.Join(specs, ","), file)
		explain = fmt.Sprintf("Операции применяются слева направо: «+» добавляет биты, «−» снимает, «=» задаёт триаду целиком, a означает все три триады. "+
			"%s → %s: %s. Итого %d. Типичная ошибка — трактовать «=» как добавление или забывать, что o= без прав обнуляет триаду.",
			permString(start), permString(mode), permDigits(mode), permOctal(mode))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Number: float64(permOctal(mode))},
		Explanation: explain,
	}, nil
}

// pipelineOutput — вывод конвейера grep/awk/sort/uniq над небольшим журналом запросов.
type pipelineOutput struct{}

func (pipelineOutput) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.linux_shell.pipeline-output.v1", Spec: itembank.SpecDevOps, Topic: "linux_shell",
		Grade: itembank.GradeMiddle, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.2, B: 0.1, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"linux", "bash", "awk"},
	}
}

var (
	logIPs      = []string{"10.0.0.12", "10.0.0.7", "10.0.1.3", "172.16.4.21", "192.168.5.9", "10.0.2.44"}
	logPaths    = []string{"/api/orders", "/api/users", "/health", "/api/items", "/login", "/metrics", "/api/cart", "/static/app.js"}
	logMethods  = []string{"GET", "GET", "GET", "POST", "PUT", "DELETE"}
	logStatuses = []int{200, 200, 200, 201, 204, 301, 400, 404, 500, 502, 503}
)

type logLine struct {
	ip, method, path string
	status, bytes    int
}

func (l logLine) String() string {
	return fmt.Sprintf("%s %s %s %d %d", l.ip, l.method, l.path, l.status, l.bytes)
}

// shellPipeline — команда и её честная реализация на Go. ok = false означает, что
// данные нужно перегенерировать (например, нет единственного максимума для head -n 1).
type shellPipeline struct {
	cmd  func(method, path string) string
	eval func(lines []logLine, method, path string) (out, explain string, ok bool)
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var shellPipelines = []shellPipeline{
	{
		cmd: func(_, _ string) string { return "awk '$4 >= 500' access.log | wc -l" },
		eval: func(lines []logLine, _, _ string) (string, string, bool) {
			n := 0
			for _, l := range lines {
				if l.status >= 500 {
					n++
				}
			}
			return strconv.Itoa(n), fmt.Sprintf("awk без блока действий печатает строки, для которых условие истинно: `$4 >= 500` — код ответа "+
				"(четвёртое поле) не меньше 500. Таких строк %d, `wc -l` печатает их число. Типичная ошибка — засчитать 4xx или вывести сами строки.", n), true
		},
	},
	{
		cmd: func(_, _ string) string { return "awk '{print $2}' access.log | sort | uniq -c | sort -rn | head -n 1" },
		eval: func(lines []logLine, _, _ string) (string, string, bool) {
			counts := map[string]int{}
			for _, l := range lines {
				counts[l.method]++
			}
			methods := sortedKeys(counts)
			best, tie := "", false
			parts := make([]string, 0, len(methods))
			for _, m := range methods {
				switch {
				case best == "" || counts[m] > counts[best]:
					best, tie = m, false
				case counts[m] == counts[best]:
					tie = true
				}
				parts = append(parts, fmt.Sprintf("%s — %d", m, counts[m]))
			}
			return fmt.Sprintf("%d %s", counts[best], best), fmt.Sprintf("`uniq -c` считает подряд идущие одинаковые строки (поэтому перед ним нужен `sort`) "+
				"и печатает число перед значением; `sort -rn` ставит самое частое первым. Частоты методов: %s. "+
				"Типичная ошибка — записать метод перед числом или взять самый редкий.", strings.Join(parts, ", ")), !tie
		},
	},
	{
		cmd: func(_, _ string) string { return "awk '{sum += $5} END {print sum}' access.log" },
		eval: func(lines []logLine, _, _ string) (string, string, bool) {
			sum := 0
			for _, l := range lines {
				sum += l.bytes
			}
			return strconv.Itoa(sum), fmt.Sprintf("Блок без условия выполняется для каждой строки и накапливает пятое поле (байты), блок END печатает сумму "+
				"один раз: %d. Типичная ошибка — суммировать коды ответов или посчитать число строк.", sum), true
		},
	},
	{
		cmd: func(method, _ string) string { return fmt.Sprintf("grep -c '%s' access.log", method) },
		eval: func(lines []logLine, method, _ string) (string, string, bool) {
			n := 0
			for _, l := range lines {
				if l.method == method {
					n++
				}
			}
			return strconv.Itoa(n), fmt.Sprintf("`grep -c` печатает число строк, содержащих образец, а не сами строки: строк с `%s` — %d. "+
				"Если совпадений нет, выводится 0. Типичная ошибка — вывести совпавшие строки.", method, n), true
		},
	},
	{
		cmd: func(_, _ string) string { return "cut -d' ' -f1 access.log | sort -u | wc -l" },
		eval: func(lines []logLine, _, _ string) (string, string, bool) {
			seen := map[string]bool{}
			for _, l := range lines {
				seen[l.ip] = true
			}
			return strconv.Itoa(len(seen)), fmt.Sprintf("`cut -f1` оставляет первое поле (IP), `sort -u` убирает повторы, `wc -l` считает оставшиеся "+
				"строки: уникальных адресов %d. Типичная ошибка — посчитать все строки журнала.", len(seen)), true
		},
	},
	{
		cmd: func(_, _ string) string { return "awk '$4 == 200 {n++} END {print n+0}' access.log" },
		eval: func(lines []logLine, _, _ string) (string, string, bool) {
			n := 0
			for _, l := range lines {
				if l.status == 200 {
					n++
				}
			}
			return strconv.Itoa(n), fmt.Sprintf("Счётчик растёт только для строк с кодом ровно 200; `n+0` в блоке END печатает 0, если таких строк "+
				"не было. Строк с кодом 200: %d. Типичная ошибка — засчитать 201 и 204 как «успешные».", n), true
		},
	},
	{
		cmd: func(_, _ string) string { return "sort -k5 -n -r access.log | head -n 1 | awk '{print $3}'" },
		eval: func(lines []logLine, _, _ string) (string, string, bool) {
			best, tie := 0, false
			for i, l := range lines {
				switch {
				case l.bytes > lines[best].bytes:
					best, tie = i, false
				case i != best && l.bytes == lines[best].bytes:
					tie = true
				}
			}
			return lines[best].path, fmt.Sprintf("`sort -k5 -n -r` сортирует по пятому полю (байты) как по числу по убыванию, `head -n 1` оставляет "+
				"самую большую запись (%d байт), awk печатает её третье поле — путь. Типичная ошибка — забыть про `-r` и взять наименьший ответ "+
				"или вывести второе поле (метод).", lines[best].bytes), !tie
		},
	},
	{
		cmd: func(_, path string) string {
			return fmt.Sprintf("grep '%s' access.log | awk '{s += $5} END {print s+0}'", path)
		},
		eval: func(lines []logLine, _, path string) (string, string, bool) {
			sum, n := 0, 0
			for _, l := range lines {
				if l.path == path {
					sum += l.bytes
					n++
				}
			}
			return strconv.Itoa(sum), fmt.Sprintf("grep оставляет только строки с путём `%s` (их %d), awk суммирует их пятое поле: %d байт; "+
				"`s+0` гарантирует вывод 0 при отсутствии строк. Типичная ошибка — суммировать байты всех строк.", path, n, sum), true
		},
	},
}

func (f pipelineOutput) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F02)
	p := itembank.Pick(r, shellPipelines)
	ips := itembank.PickN(r, logIPs, 3)
	paths := itembank.PickN(r, logPaths, 3)
	method := itembank.Pick(r, []string{"GET", "POST", "PUT", "DELETE"})
	path := itembank.Pick(r, paths)
	var lines []logLine
	var out, explain string
	for {
		n := itembank.IntBetween(r, 6, 8)
		lines = lines[:0]
		for i := 0; i < n; i++ {
			lines = append(lines, logLine{
				ip: itembank.Pick(r, ips), method: itembank.Pick(r, logMethods), path: itembank.Pick(r, paths),
				status: itembank.Pick(r, logStatuses), bytes: itembank.IntBetween(r, 100, 9999),
			})
		}
		var ok bool
		out, explain, ok = p.eval(lines, method, path)
		if ok {
			break
		}
	}
	var sb strings.Builder
	sb.WriteString("cat > access.log <<'EOF'\n")
	for _, l := range lines {
		sb.WriteString(l.String())
		sb.WriteByte('\n')
	}
	sb.WriteString("EOF\n")
	sb.WriteString(p.cmd(method, path))
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: "Скрипт выполняется в bash на Linux с GNU coreutils. Поля журнала: IP-адрес, метод, путь, код ответа, размер ответа в байтах. " +
			"Что скрипт выведет в stdout? Ведущие пробелы в строке вывода не учитываются.",
		Code:        &itembank.CodeBlock{Lang: "bash", Source: sb.String()},
		Answer:      itembank.Answer{Text: out},
		Explanation: explain,
	}, nil
}

// signalExitCode — код возврата процесса, завершённого сигналом (128 + номер).
type signalExitCode struct{}

func (signalExitCode) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.linux_shell.signal-exit-code.v1", Spec: itembank.SpecDevOps, Topic: "linux_shell",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.0, B: -0.6, C: 0}, TimeLimit: 60 * time.Second, Tags: []string{"linux", "bash"},
	}
}

// Сигналы с действием по умолчанию «завершить»; SIGINT и SIGQUIT исключены:
// фоновые процессы неинтерактивного bash их игнорируют.
var unixSignals = []struct {
	name string
	num  int
}{{"HUP", 1}, {"ABRT", 6}, {"KILL", 9}, {"USR1", 10}, {"USR2", 12}, {"PIPE", 13}, {"ALRM", 14}, {"TERM", 15}}

func (f signalExitCode) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F03)
	sig := itembank.Pick(r, unixSignals)
	pidVar := itembank.Pick(r, []string{"pid", "worker", "child", "job"})
	dur := itembank.IntBetween(r, 30, 900)
	var killCmd string
	switch r.IntN(4) {
	case 0:
		killCmd = fmt.Sprintf("kill -%s \"$%s\"", sig.name, pidVar)
	case 1:
		killCmd = fmt.Sprintf("kill -%d \"$%s\"", sig.num, pidVar)
	case 2:
		killCmd = fmt.Sprintf("kill -s SIG%s \"$%s\"", sig.name, pidVar)
	default:
		killCmd = fmt.Sprintf("kill -SIG%s \"$%s\"", sig.name, pidVar)
	}
	code := 128 + sig.num
	var echoCmd, out string
	switch r.IntN(3) {
	case 0:
		echoCmd, out = "echo $?", strconv.Itoa(code)
	case 1:
		echoCmd, out = "echo \"status=$?\"", fmt.Sprintf("status=%d", code)
	default:
		echoCmd, out = "echo \"exit code: $?\"", fmt.Sprintf("exit code: %d", code)
	}
	script := fmt.Sprintf("#!/bin/bash\nsleep %d &\n%s=$!\n%s\nwait \"$%s\"\n%s", dur, pidVar, killCmd, pidVar, echoCmd)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   "Что выведет скрипт в stdout? Сообщения bash о завершении фоновых процессов идут в stderr и не учитываются.",
		Code:   &itembank.CodeBlock{Lang: "bash", Source: script},
		Answer: itembank.Answer{Text: out},
		Explanation: fmt.Sprintf("Процесс, завершённый сигналом с номером N, получает в оболочке код возврата 128 + N: SIG%s имеет номер %d, "+
			"поэтому код равен %d. `wait` возвращает статус завершения дочернего процесса, и `$?` после него равен %d. "+
			"Типичные ошибки — считать, что код равен номеру сигнала (%d), или что `wait` вернёт 0.", sig.name, sig.num, code, code, sig.num),
	}, nil
}

// cronNextRun — следующий запуск задания по cron-выражению относительно заданного момента.
type cronNextRun struct{}

func (cronNextRun) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.linux_shell.cron-next-run.v1", Spec: itembank.SpecDevOps, Topic: "linux_shell",
		Grade: itembank.GradeSenior, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.2, B: 0.7, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"linux", "cron"},
	}
}

type cronFields struct {
	minute, hour, dom, month, dow map[int]bool
}

// parseCronField разбирает поле cron: *, */n, a-b, a,b, a-b/n.
func parseCronField(s string, lo, hi int) map[int]bool {
	out := map[int]bool{}
	for _, part := range strings.Split(s, ",") {
		step := 1
		if i := strings.IndexByte(part, '/'); i >= 0 {
			step, _ = strconv.Atoi(part[i+1:])
			part = part[:i]
		}
		a, b := lo, hi
		switch {
		case part == "*":
			// весь диапазон
		case strings.Contains(part, "-"):
			ab := strings.SplitN(part, "-", 2)
			a, _ = strconv.Atoi(ab[0])
			b, _ = strconv.Atoi(ab[1])
		default:
			a, _ = strconv.Atoi(part)
			b = a
		}
		for v := a; v <= b; v += step {
			out[v] = true
		}
	}
	return out
}

func parseCron(expr string) cronFields {
	f := strings.Fields(expr)
	return cronFields{
		minute: parseCronField(f[0], 0, 59), hour: parseCronField(f[1], 0, 23),
		dom: parseCronField(f[2], 1, 31), month: parseCronField(f[3], 1, 12), dow: parseCronField(f[4], 0, 6),
	}
}

// cronNext — первая минута строго после from, удовлетворяющая всем полям.
func cronNext(c cronFields, from time.Time) time.Time {
	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 400*24*60; i++ {
		switch {
		case !c.month[int(t.Month())] || !c.dom[t.Day()] || !c.dow[int(t.Weekday())]:
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
		case !c.hour[t.Hour()]:
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, time.UTC)
		case c.minute[t.Minute()]:
			return t
		default:
			t = t.Add(time.Minute)
		}
	}
	return t
}

// describeCronSet перечисляет значения поля (или «любое», если ограничений нет).
func describeCronSet(m map[int]bool, lo, hi int, name func(int) string) string {
	if len(m) == hi-lo+1 {
		return "любое"
	}
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for i, k := range keys {
		if i == 6 && len(keys) > 7 {
			parts = append(parts, fmt.Sprintf("… (всего %d значений)", len(keys)))
			break
		}
		parts = append(parts, name(k))
	}
	return strings.Join(parts, ", ")
}

var (
	weekdaysRU   = []string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}
	cronCommands = []string{"/usr/local/bin/backup.sh", "/opt/scripts/rotate-logs.sh", "/usr/bin/certbot renew --quiet", "/opt/app/bin/cleanup --older-than 7d", "/usr/local/bin/sync-metrics.sh", "/opt/scripts/db-vacuum.sh"}
)

func (f cronNextRun) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F04)
	m, h := r.IntN(60), r.IntN(24)
	var expr string
	switch r.IntN(6) {
	case 0:
		expr = fmt.Sprintf("%d %d * * *", m, h)
	case 1:
		expr = fmt.Sprintf("*/%d %d-%d * * 1-5", itembank.Pick(r, []int{5, 10, 15, 20, 30}), itembank.IntBetween(r, 7, 10), itembank.IntBetween(r, 16, 20))
	case 2:
		expr = fmt.Sprintf("%d */%d * * *", m, itembank.Pick(r, []int{2, 3, 4, 6, 8, 12}))
	case 3:
		expr = fmt.Sprintf("%d %d * * %d", m, h, r.IntN(7))
	case 4:
		expr = fmt.Sprintf("%d %d %d * *", m, h, itembank.IntBetween(r, 1, 28))
	default:
		ms := itembank.PickN(r, []int{0, 5, 10, 15, 20, 30, 40, 45, 50}, 2)
		hs := itembank.PickN(r, []int{6, 8, 9, 12, 14, 18, 21, 23}, 2)
		sort.Ints(ms)
		sort.Ints(hs)
		expr = fmt.Sprintf("%d,%d %d,%d * * *", ms[0], ms[1], hs[0], hs[1])
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, r.IntN(365)).
		Add(time.Duration(r.IntN(24))*time.Hour + time.Duration(r.IntN(60))*time.Minute)
	fields := parseCron(expr)
	next := cronNext(fields, now)
	num := strconv.Itoa
	explain := fmt.Sprintf("Поля: минуты — %s; часы — %s; день месяца — %s; месяц — %s; день недели — %s. "+
		"Ближайшая минута строго после текущей (%s %s), удовлетворяющая всем полям сразу, — %s (%s). "+
		"Типичные ошибки — засчитать текущую минуту, забыть, что день недели 0 — воскресенье, и что */N отсчитывается от 0, а не от текущего момента.",
		describeCronSet(fields.minute, 0, 59, num), describeCronSet(fields.hour, 0, 23, num),
		describeCronSet(fields.dom, 1, 31, num), describeCronSet(fields.month, 1, 12, num),
		describeCronSet(fields.dow, 0, 6, func(d int) string { return weekdaysRU[d] }),
		weekdaysRU[now.Weekday()], now.Format("2006-01-02 15:04"), next.Format("2006-01-02 15:04"), weekdaysRU[next.Weekday()])
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Сейчас %s, %s UTC (cron работает в UTC). В crontab есть строка ниже. Когда задание запустится в следующий раз? "+
			"Ответ в формате `ГГГГ-ММ-ДД ЧЧ:ММ`, например `2026-05-07 09:30`.", weekdaysRU[now.Weekday()], now.Format("2006-01-02 15:04")),
		Code:        &itembank.CodeBlock{Lang: "bash", Source: expr + " " + itembank.Pick(r, cronCommands)},
		Answer:      itembank.Answer{Text: next.Format("2006-01-02 15:04")},
		Explanation: explain,
	}, nil
}

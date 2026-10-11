// Package security — семейства заданий для специализации «Информационная безопасность».
package security

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(unsafeQueryFunctions{})
	itembank.Register(sameSiteCookie{})
	itembank.Register(vulnerableLine{})
	itembank.Register(validatorOutput{})
}

// unsafeQueryFunctions — какие функции доступа к БД подставляют пользовательский ввод
// в текст SQL-запроса вместо параметров драйвера.
type unsafeQueryFunctions struct{}

func (unsafeQueryFunctions) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.web_vulns.unsafe-query-functions.v1", Spec: itembank.SpecSecurity, Topic: "web_vulns",
		Grade: itembank.GradeJunior, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.4, B: -0.8, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"sql", "code-review"},
	}
}

// queryFunc — функция доступа к БД: имя, исходник ({T} — имя таблицы) и вердикт с причиной.
type queryFunc struct {
	name   string
	unsafe bool
	src    string
	why    string
}

var queryFuncs = map[string][]queryFunc{
	"go": {
		{name: "findByStatus", unsafe: true, why: "значение status склеивается с текстом запроса оператором +",
			src: "func findByStatus(db *sql.DB, status string) (*sql.Rows, error) {\n\treturn db.Query(\"SELECT id, total FROM {T} WHERE status = '\" + status + \"'\")\n}"},
		{name: "loadByID", unsafe: true, why: "fmt.Sprintf подставляет id в строку запроса до обращения к драйверу",
			src: "func loadByID(db *sql.DB, id string) *sql.Row {\n\treturn db.QueryRow(fmt.Sprintf(\"SELECT name FROM {T} WHERE id = %s\", id))\n}"},
		{name: "deleteByToken", unsafe: true, why: "запрос собирается конкатенацией в переменную q и выполняется как есть",
			src: "func deleteByToken(db *sql.DB, token string) error {\n\tq := \"DELETE FROM {T} WHERE token = '\" + token + \"'\"\n\t_, err := db.Exec(q)\n\treturn err\n}"},
		{name: "countByCity", unsafe: true, why: "значение city вставлено в строку запроса конкатенацией",
			src: "func countByCity(db *sql.DB, city string) *sql.Row {\n\treturn db.QueryRow(\"SELECT count(*) FROM {T} WHERE city = '\" + city + \"'\")\n}"},
		{name: "findByEmail", unsafe: false, why: "email передаётся отдельным параметром $1, драйвер экранирует его сам",
			src: "func findByEmail(db *sql.DB, email string) *sql.Row {\n\treturn db.QueryRow(\"SELECT id, name FROM {T} WHERE email = $1\", email)\n}"},
		{name: "searchByName", unsafe: false, why: "символы % добавляются к значению параметра, а не к тексту запроса — параметризация сохраняется",
			src: "func searchByName(db *sql.DB, q string) (*sql.Rows, error) {\n\treturn db.Query(\"SELECT id, name FROM {T} WHERE name LIKE $1\", \"%\"+q+\"%\")\n}"},
		{name: "listByOwner", unsafe: false, why: "текст запроса — константа, owner и limit идут параметрами",
			src: "func listByOwner(db *sql.DB, owner string, limit int) (*sql.Rows, error) {\n\tq := \"SELECT id FROM {T} WHERE owner = $1 ORDER BY id LIMIT $2\"\n\treturn db.Query(q, owner, limit)\n}"},
		{name: "updateNote", unsafe: false, why: "оба значения — параметры; порядок $1/$2 на безопасность не влияет",
			src: "func updateNote(db *sql.DB, id, note string) error {\n\t_, err := db.Exec(\"UPDATE {T} SET note = $2 WHERE id = $1\", id, note)\n\treturn err\n}"},
	},
	"python": {
		{name: "find_by_status", unsafe: true, why: "оператор % форматирует строку в Python ещё до вызова execute — это не плейсхолдер драйвера, хотя выглядит похоже",
			src: "def find_by_status(cur, status):\n    cur.execute(\"SELECT id, total FROM {T} WHERE status = '%s'\" % status)\n    return cur.fetchall()"},
		{name: "load_by_id", unsafe: true, why: "f-строка подставляет row_id в текст запроса до обращения к драйверу",
			src: "def load_by_id(cur, row_id):\n    cur.execute(f\"SELECT name FROM {T} WHERE id = {row_id}\")\n    return cur.fetchone()"},
		{name: "delete_by_token", unsafe: true, why: "запрос собирается конкатенацией строк",
			src: "def delete_by_token(cur, token):\n    cur.execute(\"DELETE FROM {T} WHERE token = '\" + token + \"'\")"},
		{name: "count_by_city", unsafe: true, why: "метод .format вставляет city в строку запроса",
			src: "def count_by_city(cur, city):\n    cur.execute(\"SELECT count(*) FROM {T} WHERE city = '{}'\".format(city))\n    return cur.fetchone()[0]"},
		{name: "find_by_email", unsafe: false, why: "email передаётся кортежем вторым аргументом execute — это параметр драйвера",
			src: "def find_by_email(cur, email):\n    cur.execute(\"SELECT id, name FROM {T} WHERE email = %s\", (email,))\n    return cur.fetchone()"},
		{name: "search_by_name", unsafe: false, why: "символы % добавляются к значению параметра, а не к тексту запроса — параметризация сохраняется",
			src: "def search_by_name(cur, q):\n    cur.execute(\"SELECT id, name FROM {T} WHERE name LIKE %s\", (\"%\" + q + \"%\",))\n    return cur.fetchall()"},
		{name: "list_by_owner", unsafe: false, why: "текст запроса — константа, оба значения идут параметрами",
			src: "def list_by_owner(cur, owner, limit):\n    cur.execute(\"SELECT id FROM {T} WHERE owner = %s ORDER BY id LIMIT %s\", (owner, limit))\n    return cur.fetchall()"},
		{name: "update_note", unsafe: false, why: "оба значения — параметры в кортеже",
			src: "def update_note(cur, row_id, note):\n    cur.execute(\"UPDATE {T} SET note = %s WHERE id = %s\", (note, row_id))"},
	},
	"javascript": {
		{name: "findByStatus", unsafe: true, why: "шаблонная строка подставляет status в текст запроса",
			src: "async function findByStatus(db, status) {\n  return db.query(`SELECT id, total FROM {T} WHERE status = '${status}'`);\n}"},
		{name: "loadById", unsafe: true, why: "шаблонная строка подставляет userId в текст запроса без кавычек и без параметров",
			src: "async function loadById(db, userId) {\n  return db.query(`SELECT name FROM {T} WHERE id = ${userId}`);\n}"},
		{name: "deleteByToken", unsafe: true, why: "запрос собирается конкатенацией строк",
			src: "async function deleteByToken(db, token) {\n  return db.query(\"DELETE FROM {T} WHERE token = '\" + token + \"'\");\n}"},
		{name: "countByCity", unsafe: true, why: "метод .concat — та же конкатенация, значение попадает в текст запроса",
			src: "async function countByCity(db, city) {\n  return db.query(\"SELECT count(*) FROM {T} WHERE city = '\".concat(city, \"'\"));\n}"},
		{name: "findByEmail", unsafe: false, why: "email передаётся массивом параметров, драйвер экранирует его сам",
			src: "async function findByEmail(db, email) {\n  return db.query(\"SELECT id, name FROM {T} WHERE email = $1\", [email]);\n}"},
		{name: "searchByName", unsafe: false, why: "символы % добавляются к значению параметра, а не к тексту запроса — параметризация сохраняется",
			src: "async function searchByName(db, q) {\n  return db.query(\"SELECT id, name FROM {T} WHERE name LIKE $1\", [\"%\" + q + \"%\"]);\n}"},
		{name: "listByOwner", unsafe: false, why: "текст запроса — константа, оба значения идут параметрами",
			src: "async function listByOwner(db, owner, limit) {\n  return db.query(\"SELECT id FROM {T} WHERE owner = $1 ORDER BY id LIMIT $2\", [owner, limit]);\n}"},
		{name: "updateNote", unsafe: false, why: "оба значения — параметры; порядок $1/$2 на безопасность не влияет",
			src: "async function updateNote(db, id, note) {\n  return db.query(\"UPDATE {T} SET note = $2 WHERE id = $1\", [id, note]);\n}"},
	},
}

func (f unsafeQueryFunctions) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC001)
	lang := itembank.Pick(r, []string{"go", "python", "javascript"})
	table := itembank.Ident(r, "table")
	var unsafePool, safePool []queryFunc
	for _, q := range queryFuncs[lang] {
		if q.unsafe {
			unsafePool = append(unsafePool, q)
		} else {
			safePool = append(safePool, q)
		}
	}
	k := itembank.IntBetween(r, 2, 3)
	chosen := itembank.PickN(r, unsafePool, k)
	chosen = append(chosen, itembank.PickN(r, safePool, 5-k)...)
	chosen = itembank.Shuffle(r, chosen)
	var parts, correct, wrong, badNotes, okNotes []string
	for _, q := range chosen {
		parts = append(parts, strings.ReplaceAll(q.src, "{T}", table))
		name := "`" + q.name + "`"
		if q.unsafe {
			correct = append(correct, name)
			badNotes = append(badNotes, name+" ("+q.why+")")
		} else {
			wrong = append(wrong, name)
			okNotes = append(okNotes, name+" ("+q.why+")")
		}
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	stems := []string{
		fmt.Sprintf("В коде ниже пять функций обращаются к таблице `%s`. Выберите все функции, в которых значение, пришедшее от пользователя, попадает в текст SQL-запроса без параметризации (уязвимость SQL-инъекции).", table),
		fmt.Sprintf("Проведите ревью фрагмента: отметьте все функции, где запрос к таблице `%s` собирается из пользовательского ввода конкатенацией или форматированием строки, а не передаёт значения параметрами драйвера.", table),
		"Какие из функций ниже уязвимы к SQL-инъекции? Выберите все подходящие: уязвимой считается функция, в которой входные данные становятся частью строки запроса до передачи в БД.",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: lang, Source: strings.Join(parts, "\n\n")},
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: fmt.Sprintf("Уязвимы: %s. Безопасны: %s. Параметризованный запрос передаёт значения отдельно от текста SQL, и драйвер не позволяет им изменить структуру запроса. "+
			"Ловушки: оператор %% в Python форматирует строку до execute, хотя выглядит как плейсхолдер; символы %% вокруг значения для LIKE добавляются к параметру, а не к запросу, поэтому это безопасно.",
			strings.Join(badNotes, "; "), strings.Join(okNotes, "; ")),
	}, nil
}

// sameSiteCookie — к каким запросам браузер приложит cookie с заданными атрибутами SameSite и Secure.
type sameSiteCookie struct{}

func (sameSiteCookie) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.web_vulns.samesite-cookie.v1", Spec: itembank.SpecSecurity, Topic: "web_vulns",
		Grade: itembank.GradeMiddle, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.4, B: 0.3, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"http", "cookies", "csrf", "browser"},
	}
}

// cookieCase — сценарий запроса: описание, при каких значениях SameSite cookie уходит, и почему.
type cookieCase struct {
	text string
	sent map[string]bool
	why  string
}

func cookieCases(svc, other string) []cookieCase {
	api := "https://api." + svc + ".example"
	app := "https://app." + svc + ".example"
	ext := "https://" + other + ".example"
	all := map[string]bool{"Strict": true, "Lax": true, "None": true}
	never := map[string]bool{}
	laxNone := map[string]bool{"Lax": true, "None": true}
	onlyNone := map[string]bool{"None": true}
	return []cookieCase{
		{"Пользователь на странице `" + app + "/cart` переходит по ссылке на `" + api + "/orders`", all,
			"запрос same-site (общий регистрируемый домен " + svc + ".example), атрибут SameSite его не ограничивает"},
		{"Скрипт на `" + app + "` вызывает `fetch(\"" + api + "/me\", {credentials: \"include\"})`", all,
			"same-site запрос с явным credentials: include — cookie прикладывается при любом SameSite"},
		{"Форма на `" + app + "/settings` отправляется методом POST на `" + api + "/profile`", all,
			"same-site POST: ограничения SameSite касаются только кросс-сайтовых запросов"},
		{"Пользователь на `" + ext + "/promo` нажимает ссылку на `" + api + "/orders`", laxNone,
			"кросс-сайтовая навигация верхнего уровня методом GET: Lax и None разрешают, Strict — нет"},
		{"Страница `" + ext + "` отправляет скрытую форму методом POST на `" + api + "/profile`", onlyNone,
			"кросс-сайтовый POST: Lax отправляет cookie только при навигации безопасным методом, Strict — никогда, None — всегда"},
		{"Скрипт на `" + ext + "` вызывает `fetch(\"" + api + "/me\", {credentials: \"include\"})`", onlyNone,
			"кросс-сайтовый подзапрос из скрипта: cookie уходит только при SameSite=None"},
		{"На странице `" + ext + "` стоит `<img src=\"" + api + "/avatar.png\">`", onlyNone,
			"загрузка подресурса с другого сайта — кросс-сайтовый запрос, разрешён только при None"},
		{"Скрипт на `" + ext + "` вызывает `fetch(\"" + api + "/me\")` без параметра credentials", never,
			"по умолчанию fetch использует credentials: same-origin и не прикладывает cookie к запросам на другой origin независимо от SameSite"},
		{"Страница `http://app." + svc + ".example` (без TLS) делает GET на `http://api." + svc + ".example/orders`", never,
			"атрибут Secure запрещает отправку cookie по незашифрованному HTTP"},
		{"Скрипт на `" + app + "` вызывает `fetch(\"https://cdn." + svc + ".example/assets\", {credentials: \"include\"})`", never,
			"cookie установлена без атрибута Domain и привязана к хосту api." + svc + ".example — на cdn она не отправляется"},
	}
}

func (f sameSiteCookie) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC002)
	services := itembank.Idents(r, "service", 2)
	svc := strings.ReplaceAll(services[0], "-", "")
	other := strings.ReplaceAll(services[1], "-", "")
	mode := itembank.Pick(r, []string{"Strict", "Lax", "None"})
	var yes, no []cookieCase
	for _, c := range cookieCases(svc, other) {
		if c.sent[mode] {
			yes = append(yes, c)
		} else {
			no = append(no, c)
		}
	}
	k := itembank.IntBetween(r, 2, 3)
	if k > len(yes) {
		k = len(yes)
	}
	if 5-k > len(no) {
		k = 5 - len(no)
	}
	chosen := itembank.PickN(r, yes, k)
	chosen = append(chosen, itembank.PickN(r, no, 5-k)...)
	chosen = itembank.Shuffle(r, chosen)
	var correct, wrong, notes []string
	for _, c := range chosen {
		verdict := "не отправится"
		if c.sent[mode] {
			correct = append(correct, c.text)
			verdict = "отправится"
		} else {
			wrong = append(wrong, c.text)
		}
		notes = append(notes, fmt.Sprintf("«%s» — %s: %s", c.text, verdict, c.why))
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	extra := ""
	if r.IntN(2) == 0 {
		extra = fmt.Sprintf(" Max-Age=%d;", itembank.Pick(r, []int{1800, 3600, 86400}))
	}
	header := fmt.Sprintf("HTTP/1.1 200 OK\nSet-Cookie: sid=%08x%08x; Path=/;%s Secure; HttpOnly; SameSite=%s", r.Uint32(), r.Uint32(), extra, mode)
	stems := []string{
		fmt.Sprintf("Ответ на запрос к `https://api.%s.example/login` установил cookie заголовком ниже (атрибут Domain не задан). Выберите все запросы, к которым браузер приложит эту cookie.", svc),
		fmt.Sprintf("Сервер `api.%s.example` выставил сессионную cookie (ответ ниже; Domain не указан, поэтому cookie привязана к этому хосту). Отметьте все ситуации, в которых браузер отправит cookie вместе с запросом.", svc),
		fmt.Sprintf("Пользователь вошёл на `https://api.%s.example` и получил cookie из ответа ниже. В каких из перечисленных случаев браузер включит её в запрос? Выберите все верные.", svc),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Code:    &itembank.CodeBlock{Lang: "http", Source: header},
		Options: options,
		Answer:  itembank.Answer{Keys: keys},
		Explanation: fmt.Sprintf("SameSite=%s. Same-site — это общий регистрируемый домен (%s.example) и схема; Strict не отправляет cookie ни с одним кросс-сайтовым запросом, "+
			"Lax — только с навигацией верхнего уровня безопасным методом, None — со всеми (при обязательном Secure). Разбор: %s. "+
			"Типичные ошибки — считать запрос с поддомена кросс-сайтовым, забывать про Secure и про то, что fetch без credentials cookie не шлёт.",
			mode, svc, strings.Join(notes, "; ")),
	}, nil
}

// vulnerableLine — номер строки с уязвимостью во фрагменте обработчика смены пароля.
type vulnerableLine struct{}

func (vulnerableLine) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.secure_code_review.vulnerable-line.v1", Spec: itembank.SpecSecurity, Topic: "secure_code_review",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.3, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"go", "python", "javascript", "owasp"},
	}
}

// reviewKind — класс уязвимости: уязвимая строка и её безопасный аналог для каждого языка.
type reviewKind struct {
	name    string
	title   string // о чём строка, для перечисления безопасных строк
	vuln    map[string]string
	safe    map[string]string
	why     string
	safeWhy string
}

var reviewKinds = []reviewKind{
	{
		name: "cmd", title: "вызов внешней команды",
		vuln: map[string]string{
			"go":         `_ = exec.Command("sh", "-c", "logger password-changed "+user).Run()`,
			"python":     `subprocess.run("logger password-changed " + user, shell=True)`,
			"javascript": `exec("logger password-changed " + user);`,
		},
		safe: map[string]string{
			"go":         `_ = exec.Command("logger", "-t", "auth", "password-changed "+user).Run()`,
			"python":     `subprocess.run(["logger", "-t", "auth", "password-changed " + user], check=False)`,
			"javascript": `execFile("logger", ["-t", "auth", "password-changed " + user]);`,
		},
		why:     "команда собирается строкой и выполняется через оболочку, поэтому спецсимволы в имени пользователя меняют саму команду — внедрение команд (command injection)",
		safeWhy: "аргументы передаются массивом без оболочки, ввод остаётся одним аргументом",
	},
	{
		name: "log", title: "журналирование",
		vuln: map[string]string{
			"go":         `log.Printf("password change: user=%s new=%s", user, password)`,
			"python":     `logger.info("password change user=%s new=%s", user, password)`,
			"javascript": "console.log(`password change user=${user} new=${password}`);",
		},
		safe: map[string]string{
			"go":         `log.Printf("password change: user=%s ip=%s", user, r.RemoteAddr)`,
			"python":     `logger.info("password change user=%s ip=%s", user, request.remote_addr)`,
			"javascript": "console.log(`password change user=${user} ip=${req.ip}`);",
		},
		why:     "новый пароль записывается в журнал в открытом виде — утечка секрета через логи и системы их агрегации",
		safeWhy: "в журнал попадают только имя пользователя и адрес",
	},
	{
		name: "hash", title: "хеширование пароля",
		vuln: map[string]string{
			"go":         `s.store.SavePasswordHash(ctx, user, fmt.Sprintf("%x", md5.Sum([]byte(password))))`,
			"python":     `store.save_password_hash(user, hashlib.md5(password.encode()).hexdigest())`,
			"javascript": `await store.savePasswordHash(user, crypto.createHash("md5").update(password).digest("hex"));`,
		},
		safe: map[string]string{
			"go":         `s.store.SavePasswordHash(ctx, user, hashArgon2id(password))`,
			"python":     `store.save_password_hash(user, argon2.hash(password))`,
			"javascript": `await store.savePasswordHash(user, await argon2.hash(password));`,
		},
		why:     "пароль хешируется быстрым MD5 без соли — такие хеши подбираются перебором; нужен медленный KDF (Argon2id, bcrypt, scrypt)",
		safeWhy: "используется медленная функция Argon2id с солью",
	},
	{
		name: "tls", title: "исходящий HTTP-запрос",
		vuln: map[string]string{
			"go":         `http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}`,
			"python":     `requests.post(NOTIFY_URL, json={"user": user}, verify=False)`,
			"javascript": `process.env.NODE_TLS_REJECT_UNAUTHORIZED = "0";`,
		},
		safe: map[string]string{
			"go":         `http.DefaultClient.Timeout = 3 * time.Second`,
			"python":     `requests.post(NOTIFY_URL, json={"user": user}, timeout=3)`,
			"javascript": `await fetch(NOTIFY_URL, { method: "POST", body: JSON.stringify({ user }) });`,
		},
		why:     "отключена проверка TLS-сертификата сервера — соединение можно перехватить и подменить (MITM)",
		safeWhy: "проверка сертификата не отключена, задан лишь таймаут или обычный запрос",
	},
	{
		name: "secret", title: "подключение к почтовому сервису",
		vuln: map[string]string{
			"go":         `s.mailer.Login(ctx, "notify", "Sup3rSecret!")`,
			"python":     `mailer.login("notify", "Sup3rSecret!")`,
			"javascript": `await mailer.login("notify", "Sup3rSecret!");`,
		},
		safe: map[string]string{
			"go":         `s.mailer.Login(ctx, "notify", os.Getenv("SMTP_PASSWORD"))`,
			"python":     `mailer.login("notify", os.environ["SMTP_PASSWORD"])`,
			"javascript": `await mailer.login("notify", process.env.SMTP_PASSWORD);`,
		},
		why:     "учётные данные зашиты в исходный код — попадают в репозиторий и сборки; их нужно читать из окружения или хранилища секретов",
		safeWhy: "пароль берётся из переменной окружения",
	},
	{
		name: "rand", title: "токен сброса пароля",
		vuln: map[string]string{
			"go":         `s.store.SaveResetToken(ctx, user, strconv.Itoa(rand.Intn(1000000)))`,
			"python":     `store.save_reset_token(user, str(random.randint(0, 999999)))`,
			"javascript": `await store.saveResetToken(user, String(Math.floor(Math.random() * 1e6)));`,
		},
		safe: map[string]string{
			"go":         `s.store.SaveResetToken(ctx, user, randomToken(32))`,
			"python":     `store.save_reset_token(user, secrets.token_urlsafe(32))`,
			"javascript": `await store.saveResetToken(user, crypto.randomBytes(32).toString("hex"));`,
		},
		why:     "токен сброса берётся из математического, а не криптографического генератора и к тому же короткий — его можно предсказать или перебрать",
		safeWhy: "длинный токен из криптографического генератора",
	},
	{
		name: "html", title: "вывод HTML",
		vuln: map[string]string{
			"go":         `fmt.Fprintf(w, "<p>Password updated for %s</p>", user)`,
			"python":     `flash(Markup(f"<p>Password updated for {user}</p>"))`,
			"javascript": "res.write(`<p>Password updated for ${user}</p>`);",
		},
		safe: map[string]string{
			"go":         `fmt.Fprintf(w, "<p>Password updated for %s</p>", html.EscapeString(user))`,
			"python":     `flash(f"<p>Password updated for {escape(user)}</p>")`,
			"javascript": "res.write(`<p>Password updated for ${escapeHtml(user)}</p>`);",
		},
		why:     "имя пользователя вставляется в HTML без экранирования — отражённый XSS",
		safeWhy: "значение экранируется перед вставкой в разметку",
	},
	{
		name: "deser", title: "разбор пользовательских настроек",
		vuln: map[string]string{
			"python":     `prefs = pickle.loads(base64.b64decode(request.form["prefs"]))`,
			"javascript": `const prefs = eval("(" + req.body.prefs + ")");`,
		},
		safe: map[string]string{
			"python":     `prefs = json.loads(request.form["prefs"])`,
			"javascript": `const prefs = JSON.parse(req.body.prefs);`,
		},
		why:     "данные от клиента десериализуются или исполняются как код — небезопасная десериализация даёт выполнение произвольного кода",
		safeWhy: "используется JSON-разбор без выполнения кода",
	},
}

// reviewFrame — обрамление функции: строки до и после проверяемого блока и отступ тела.
type reviewFrame struct {
	head, tail []string
	indent     string
}

var reviewFrames = map[string]reviewFrame{
	"go": {indent: "\t",
		head: []string{
			`func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {`,
			"\t" + `user := strings.TrimSpace(r.FormValue("user"))`,
			"\t" + `password := r.FormValue("new_password")`,
			"\t" + `ctx := r.Context()`,
		},
		tail: []string{"\t" + `s.audit.Record(ctx, "password_changed", user)`, `}`},
	},
	"python": {indent: "    ",
		head: []string{
			`@app.route("/account/password", methods=["POST"])`,
			`def change_password():`,
			`    user = request.form["user"].strip()`,
			`    password = request.form["new_password"]`,
		},
		tail: []string{`    sessions.revoke_all(user)`, `    return "", 204`},
	},
	"javascript": {indent: "  ",
		head: []string{
			`app.post("/account/password", async (req, res) => {`,
			`  const user = String(req.body.user).trim();`,
			`  const password = String(req.body.newPassword);`,
		},
		tail: []string{`  await sessions.revokeAll(user);`, `  res.end();`, `});`},
	},
}

func (f vulnerableLine) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC003)
	lang := itembank.Pick(r, []string{"go", "python", "javascript"})
	var kinds []reviewKind
	for _, k := range reviewKinds {
		if _, ok := k.vuln[lang]; ok {
			kinds = append(kinds, k)
		}
	}
	vIdx := r.IntN(len(kinds))
	v := kinds[vIdx]
	// Хеширование присутствует всегда (это обработчик смены пароля), остальные классы — случайны.
	var pool, chosen []reviewKind
	for i, k := range kinds {
		switch {
		case i == vIdx:
		case k.name == "hash":
			chosen = append(chosen, k)
		default:
			pool = append(pool, k)
		}
	}
	chosen = append(chosen, itembank.PickN(r, pool, 4-len(chosen))...)
	frame := reviewFrames[lang]
	middle := []string{v.vuln[lang]}
	for _, k := range chosen {
		middle = append(middle, k.safe[lang])
	}
	middle = itembank.Shuffle(r, middle)
	lines := append([]string{}, frame.head...)
	answer := 0
	for i, m := range middle {
		if m == v.vuln[lang] {
			answer = len(frame.head) + i + 1
		}
		lines = append(lines, frame.indent+m)
	}
	lines = append(lines, frame.tail...)
	var safeNotes []string
	for _, k := range chosen {
		safeNotes = append(safeNotes, k.title+" — "+k.safeWhy)
	}
	stems := []string{
		"Ревью кода: во фрагменте ниже ровно одна строка содержит уязвимость (внедрение команд, утечка секрета в журнал, слабое хеширование пароля, отключённая проверка TLS, секрет в коде, предсказуемый токен, XSS или небезопасная десериализация). Укажите номер этой строки. Строки нумеруются с 1, первая строка фрагмента — номер 1.",
		"Перед вами обработчик смены пароля. Одна из строк создаёт уязвимость из списка OWASP (инъекция команд, утечка пароля в лог, быстрый хеш для пароля, отключённая проверка сертификата, зашитый секрет, небезопасный генератор токена, XSS, небезопасная десериализация). В какой строке она находится? Нумерация с 1 от первой строки фрагмента.",
		"Найдите во фрагменте строку, которую нельзя пропускать в продакшен: ровно одна из них содержит типичную уязвимость (command injection, секрет в логе, MD5 для пароля, InsecureSkipVerify/verify=False, пароль в коде, Math.random/random для токена, вывод HTML без экранирования, pickle/eval). Ответ — номер строки, считая первую строку фрагмента первой.",
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: lang, Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Уязвима строка %d: `%s` — %s. Остальные строки блока безопасны: %s. "+
			"Типичная ошибка — помечать любую похожую конструкцию (вызов команды, запись в журнал, хеширование) не глядя на детали: решает именно способ передачи данных.",
			answer, strings.TrimSpace(v.vuln[lang]), v.why, strings.Join(safeNotes, "; ")),
	}, nil
}

// validatorOutput — что напечатает программа, проверяющая имена регулярным выражением
// с неполными якорями или неверно выбранным методом поиска.
type validatorOutput struct{}

func (validatorOutput) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.secure_code_review.validator-output.v1", Spec: itembank.SpecSecurity, Topic: "secure_code_review",
		Grade: itembank.GradeSenior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.6, B: 0.9, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"regex", "validation", "go", "python", "javascript"},
	}
}

var (
	validatorInputs = []string{"alice", "Bob", "x", "root_1", "admin;", "us er", "verylongname9", "_tmp", "9lives", "ab", "ok_42", "a-b"}
	validatorBodies = []string{"[a-z][a-z0-9_]{2,7}", "[a-z0-9_]{3,8}"}
)

func regexAccepted(re *regexp.Regexp, inputs []string) []string {
	out := []string{}
	for _, s := range inputs {
		if re.MatchString(s) {
			out = append(out, s)
		}
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func quoteList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = `"` + x + `"`
	}
	return strings.Join(q, ", ")
}

func (f validatorOutput) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC004)
	lang := itembank.Pick(r, []string{"go", "python", "javascript"})
	body := itembank.Pick(r, validatorBodies)
	anchors := itembank.Pick(r, []string{"both", "start", "end", "none"})
	method := "test"
	switch lang {
	case "python":
		method = itembank.Pick(r, []string{"match", "match", "search", "fullmatch"})
	case "go":
		method = "MatchString"
	}
	pat := body
	switch anchors {
	case "both":
		pat = "^" + body + "$"
	case "start":
		pat = "^" + body
	case "end":
		pat = body + "$"
	}
	// Эмуляция семантики: re.match неявно закрепляет начало, re.fullmatch — обе стороны.
	eff := pat
	switch method {
	case "match":
		eff = "^(?:" + pat + ")"
	case "fullmatch":
		eff = "^(?:" + pat + ")$"
	}
	re := regexp.MustCompile(eff)
	strict := regexp.MustCompile("^(?:" + body + ")$")
	strictEquivalent := sameStrings(regexAccepted(re, validatorInputs), regexAccepted(strict, validatorInputs))
	var candidates, ok, okStrict []string
	for try := 0; try < 60; try++ {
		candidates = itembank.PickN(r, validatorInputs, 6)
		ok = regexAccepted(re, candidates)
		okStrict = regexAccepted(strict, candidates)
		if len(ok) >= 1 && len(ok) <= 5 && (strictEquivalent || !sameStrings(ok, okStrict)) {
			break
		}
	}
	if len(ok) == 0 {
		candidates[0] = "alice"
		ok = regexAccepted(re, candidates)
		okStrict = regexAccepted(strict, candidates)
	}
	var code, runtime string
	switch lang {
	case "python":
		runtime = "Python 3"
		code = fmt.Sprintf("import re\n\nPATTERN = re.compile(r\"%s\")\n\n\ndef is_valid(name):\n    return PATTERN.%s(name) is not None\n\n\ncandidates = [%s]\nok = [c for c in candidates if is_valid(c)]\nprint(len(ok), \",\".join(ok))", pat, method, quoteList(candidates))
	case "javascript":
		runtime = "Node.js"
		code = fmt.Sprintf("const pattern = /%s/;\n\nfunction isValid(name) {\n  return pattern.test(name);\n}\n\nconst candidates = [%s];\nconst ok = candidates.filter(isValid);\nconsole.log(ok.length, ok.join(\",\"));", pat, quoteList(candidates))
	default:
		runtime = "Go"
		code = fmt.Sprintf("package main\n\nimport (\n\t\"fmt\"\n\t\"regexp\"\n\t\"strings\"\n)\n\nvar pattern = regexp.MustCompile(`%s`)\n\nfunc isValid(name string) bool {\n\treturn pattern.MatchString(name)\n}\n\nfunc main() {\n\tcandidates := []string{%s}\n\tvar ok []string\n\tfor _, c := range candidates {\n\t\tif isValid(c) {\n\t\t\tok = append(ok, c)\n\t\t}\n\t}\n\tfmt.Println(len(ok), strings.Join(ok, \",\"))\n}", pat, quoteList(candidates))
	}
	var semantics string
	switch method {
	case "match":
		semantics = "`re.match` неявно закрепляет шаблон у начала строки (как `^`), но не у конца"
	case "fullmatch":
		semantics = "`re.fullmatch` требует совпадения всей строки независимо от явных `^` и `$`"
	case "search":
		semantics = "`re.search` ищет совпадение в любом месте строки; закрепляют его только явные `^` и `$`"
	case "test":
		semantics = "`RegExp.test` ищет совпадение в любом месте строки; закрепляют его только явные `^` и `$`"
	default:
		semantics = "`MatchString` ищет совпадение в любом месте строки; закрепляют его только явные `^` и `$`"
	}
	anchorNote := map[string]string{
		"both":  "в шаблоне есть оба якоря",
		"start": "в шаблоне есть только `^`, поэтому проходит любая строка с допустимым префиксом",
		"end":   "в шаблоне есть только `$`, поэтому проходит любая строка с допустимым суффиксом",
		"none":  "якорей нет, достаточно допустимой подстроки",
	}[anchors]
	verdict := "проверка нестрогая"
	if strictEquivalent {
		verdict = "итоговая проверка строгая"
	}
	var rejected []string
	for _, c := range candidates {
		found := false
		for _, o := range ok {
			if o == c {
				found = true
			}
		}
		if !found {
			rejected = append(rejected, c)
		}
	}
	stems := []string{
		fmt.Sprintf("Программа на %s проверяет имена пользователей регулярным выражением и печатает число принятых имён и сами имена через запятую. Что будет выведено?", runtime),
		fmt.Sprintf("Фрагмент на %s — валидатор логинов из формы регистрации. Что он выведет при запуске? Ответ — одна строка вывода целиком.", runtime),
		fmt.Sprintf("Ниже код проверки имён (%s). Определите, какие из кандидатов пройдут проверку, и напишите ровно то, что программа напечатает.", runtime),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   itembank.Pick(r, stems),
		Code:   &itembank.CodeBlock{Lang: lang, Source: code},
		Answer: itembank.Answer{Text: fmt.Sprintf("%d %s", len(ok), strings.Join(ok, ","))},
		Explanation: fmt.Sprintf("%s; %s — %s. Приняты: %s (%d). Отклонены: %s. При строгой проверке `^%s$` прошли бы только: %s. "+
			"Типичная ошибка ревью — считать, что `match`/`test` проверяет всю строку: без якоря конца через валидатор проходят имена с запрещённым хвостом или превышающие длину.",
			semantics, anchorNote, verdict, strings.Join(ok, ", "), len(ok), strings.Join(rejected, ", "), body, strings.Join(okStrict, ", ")),
	}, nil
}

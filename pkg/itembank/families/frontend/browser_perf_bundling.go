package frontend

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(urlSearchParamsOutput{})
	itembank.Register(webStorageOutput{})
	itembank.Register(forcedReflowCount{})
	itembank.Register(useMemoRecompute{})
	itembank.Register(lazyChunksBytes{})
}

// urlSearchParamsOutput — результат операций URLSearchParams: set заменяет первое вхождение и убирает дубликаты,
// append добавляет, delete удаляет все, get возвращает первое значение, порядок пар сохраняется.
type urlSearchParamsOutput struct{}

func (urlSearchParamsOutput) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.browser_api.url-search-params.v1", Spec: itembank.SpecFrontend, Topic: "browser_api",
		Grade: itembank.GradeJunior, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.4, B: -0.7, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"javascript", "url", "web-api"},
	}
}

type kvPair struct{ k, v string }

func kvString(ps []kvPair) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
}

func (f urlSearchParamsOutput) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF601)
	keys := itembank.PickN(r, []string{"page", "sort", "tag", "q", "limit", "lang", "id", "filter"}, 4)
	newKey := keys[3]
	distinct := keys[:itembank.IntBetween(r, 2, 3)]
	values := itembank.Shuffle(r, []string{"asc", "desc", "new", "top", "en", "ru", "go", "js", "2", "5", "7", "10", "20", "42"})
	nextVal := 0
	val := func() string {
		v := values[nextVal%len(values)]
		nextVal++
		return v
	}
	var pairs []kvPair
	for _, k := range distinct {
		pairs = append(pairs, kvPair{k, val()})
	}
	if r.IntN(10) < 7 { // дубликат ключа — на нём видна разница set/append и get/getAll
		pairs = append(pairs, kvPair{itembank.Pick(r, distinct), val()})
	}
	pairs = itembank.Shuffle(r, pairs)
	initial := kvString(pairs)
	seen := map[string]bool{}
	for _, p := range pairs {
		seen[p.k] = true
	}

	var code []string
	if r.IntN(2) == 0 {
		code = append(code, fmt.Sprintf("const params = new URLSearchParams('%s');", initial))
	} else {
		host := itembank.Pick(r, []string{"https://shop.test/items", "https://app.example.com/list", "https://news.test/feed"})
		code = append(code, fmt.Sprintf("const params = new URL('%s?%s').searchParams;", host, initial))
	}
	var trace []string
	usedDelete, usedSort := false, false
	for i := itembank.IntBetween(r, 2, 3); i > 0; i-- {
		kind := r.IntN(10)
		key := itembank.Pick(r, distinct)
		if r.IntN(10) < 3 {
			key = newKey
		}
		switch {
		case kind < 4: // append
			v := val()
			pairs = append(pairs, kvPair{key, v})
			seen[key] = true
			code = append(code, fmt.Sprintf("params.append('%s', '%s');", key, v))
			trace = append(trace, fmt.Sprintf("`append('%s', '%s')` → `%s`", key, v, kvString(pairs)))
		case kind < 7: // set
			v := val()
			var out []kvPair
			replaced := false
			for _, p := range pairs {
				if p.k != key {
					out = append(out, p)
				} else if !replaced {
					out = append(out, kvPair{key, v})
					replaced = true
				}
			}
			if !replaced {
				out = append(out, kvPair{key, v})
			}
			pairs = out
			seen[key] = true
			code = append(code, fmt.Sprintf("params.set('%s', '%s');", key, v))
			trace = append(trace, fmt.Sprintf("`set('%s', '%s')` → `%s`", key, v, kvString(pairs)))
		case kind < 9 && !usedDelete:
			usedDelete = true
			key = itembank.Pick(r, distinct)
			var out []kvPair
			for _, p := range pairs {
				if p.k != key {
					out = append(out, p)
				}
			}
			pairs = out
			code = append(code, fmt.Sprintf("params.delete('%s');", key))
			trace = append(trace, fmt.Sprintf("`delete('%s')` → `%s`", key, kvString(pairs)))
		case !usedSort:
			usedSort = true
			sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
			code = append(code, "params.sort();")
			trace = append(trace, fmt.Sprintf("`sort()` → `%s`", kvString(pairs)))
		default:
			i++ // операция не выбрана, пробуем ещё раз
		}
	}

	var present []string
	presentSeen := map[string]bool{}
	for _, p := range pairs {
		if !presentSeen[p.k] {
			presentSeen[p.k] = true
			present = append(present, p.k)
		}
	}
	var allKeys []string
	for _, k := range keys {
		if seen[k] {
			allKeys = append(allKeys, k)
		}
	}
	var answer, rule string
	switch q := r.IntN(10); {
	case q < 4:
		answer = kvString(pairs)
		code = append(code, itembank.Pick(r, []string{"console.log(params.toString());", "console.log(String(params));"}))
		rule = "`toString()` сериализует пары в текущем порядке — без сортировки и без склейки дубликатов."
	case q < 7:
		k := itembank.Pick(r, allKeys)
		answer = "null"
		for _, p := range pairs {
			if p.k == k {
				answer = p.v
				break
			}
		}
		code = append(code, fmt.Sprintf("console.log(params.get('%s'));", k))
		rule = fmt.Sprintf("`get('%s')` возвращает значение первого вхождения ключа, а для отсутствующего ключа — `null`.", k)
	case q < 9:
		k := itembank.Pick(r, present)
		var vals []string
		for _, p := range pairs {
			if p.k == k {
				vals = append(vals, p.v)
			}
		}
		answer = strings.Join(vals, ",")
		code = append(code, fmt.Sprintf("console.log(params.getAll('%s').join(','));", k))
		rule = fmt.Sprintf("`getAll('%s')` возвращает все значения ключа в порядке следования, а `get` вернул бы только первое.", k)
	default:
		k := itembank.Pick(r, allKeys)
		answer = fmt.Sprintf("%t", presentSeen[k])
		code = append(code, fmt.Sprintf("console.log(params.has('%s'));", k))
		rule = fmt.Sprintf("`has('%s')` проверяет наличие хотя бы одной пары с этим ключом после всех операций.", k)
	}
	stem := itembank.Pick(r, []string{
		"Что выведет код в консоль?",
		"Какая строка будет напечатана в консоли?",
		"Что покажет `console.log` в последней строке?",
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(code, "\n")},
		Answer: itembank.Answer{Text: answer},
		Explanation: fmt.Sprintf("Исходные пары: `%s`. Операции: %s. `append` добавляет пару в конец, `set` заменяет значение первого вхождения "+
			"и удаляет остальные дубликаты ключа, `delete` удаляет все вхождения, `sort()` устойчиво сортирует по ключу. %s Итог: `%s`.",
			initial, strings.Join(trace, "; "), rule, answer),
	}, nil
}

// webStorageOutput — что выведет код, читающий localStorage/sessionStorage после перезагрузки или в новой вкладке:
// хранилища хранят строки, sessionStorage живёт в пределах вкладки, отсутствующий ключ даёт null.
type webStorageOutput struct{}

func (webStorageOutput) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.browser_api.web-storage-output.v1", Spec: itembank.SpecFrontend, Topic: "browser_api",
		Grade: itembank.GradeMiddle, Format: itembank.FormatCodeOutput,
		IRT: itembank.IRTParams{A: 1.5, B: 0.0, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"javascript", "web-storage"},
	}
}

type storageScenario struct {
	stem, comment   string
	sessionSurvives bool
	why             string
}

var storageScenarios = []storageScenario{
	{"затем пользователь перезагружает вкладку A, и после загрузки", "// вкладка A после перезагрузки", true,
		"перезагрузка не очищает `sessionStorage` — он привязан к вкладке, а не к загрузке страницы"},
	{"затем пользователь открывает тот же сайт в новой вкладке B, где", "// вкладка B (новая вкладка, тот же origin)", false,
		"новая вкладка получает пустой `sessionStorage`: он не разделяется между вкладками даже одного origin"},
	{"затем пользователь закрывает вкладку A, позже снова открывает сайт в новой вкладке, где", "// новая вкладка после закрытия A", false,
		"с закрытием вкладки её `sessionStorage` уничтожается, а новая вкладка начинает с пустого"},
	{"и сразу после этого в той же вкладке A", "// вкладка A, продолжение скрипта", true,
		"в той же вкладке `sessionStorage` доступен целиком"},
}

func (f webStorageOutput) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF602)
	key := itembank.Pick(r, []string{"visits", "count", "step", "score", "level"})
	n, m, d := itembank.IntBetween(r, 1, 9), itembank.IntBetween(r, 1, 9), itembank.IntBetween(r, 1, 9)
	for m == n {
		m = itembank.IntBetween(r, 1, 9)
	}
	v := itembank.Pick(r, []string{"v", "raw", "saved", "prev"})
	sc := itembank.Pick(r, storageScenarios)

	codeA := []string{
		"// вкладка A",
		fmt.Sprintf("localStorage.setItem('%s', %d);", key, n),
		fmt.Sprintf("sessionStorage.setItem('%s', %d);", key, m),
	}
	var local, opWhy string
	localNull := false
	switch r.IntN(7) {
	case 0, 1:
		codeA = append(codeA, fmt.Sprintf("const %s = localStorage.getItem('%s');", v, key), fmt.Sprintf("localStorage.setItem('%s', %s + %d);", key, v, d))
		local = fmt.Sprintf("%d%d", n, d)
		opWhy = fmt.Sprintf("`getItem` вернул строку `\"%d\"`, поэтому `%s + %d` — конкатенация, а не сложение: сохраняется `\"%s\"`", n, v, d, local)
	case 2, 3:
		codeA = append(codeA, fmt.Sprintf("const %s = localStorage.getItem('%s');", v, key), fmt.Sprintf("localStorage.setItem('%s', Number(%s) + %d);", key, v, d))
		local = fmt.Sprintf("%d", n+d)
		opWhy = fmt.Sprintf("`Number(%s) + %d` даёт число %s, которое `setItem` снова сохраняет строкой", v, d, local)
	case 4:
		codeA = append(codeA, fmt.Sprintf("const %s = localStorage.getItem('%s');", v, key), fmt.Sprintf("localStorage.setItem('%s', %s * %d);", key, v, d))
		local = fmt.Sprintf("%d", n*d)
		opWhy = fmt.Sprintf("в отличие от `+`, оператор `*` приводит строку `\"%d\"` к числу: сохраняется `\"%s\"`", n, local)
	default:
		codeA = append(codeA, fmt.Sprintf("localStorage.removeItem('%s');", key))
		local, localNull = "null", true
		opWhy = fmt.Sprintf("`removeItem('%s')` удалил запись, и `getItem` для неё возвращает `null`", key)
	}
	session := fmt.Sprintf("%d", m)
	if !sc.sessionSurvives {
		session = "null"
	}

	var finalLine, first, formWhy string
	switch r.IntN(3) {
	case 0:
		finalLine = fmt.Sprintf("console.log(localStorage.getItem('%s'), sessionStorage.getItem('%s'));", key, key)
		first = local
		formWhy = "`console.log` печатает строки без кавычек"
	case 1:
		finalLine = fmt.Sprintf("console.log(typeof localStorage.getItem('%s'), sessionStorage.getItem('%s'));", key, key)
		first = "string"
		formWhy = "любое значение из Web Storage имеет тип `string`"
		if localNull {
			first = "object"
			formWhy = "`typeof null` — исторически `\"object\"`"
		}
	default:
		finalLine = fmt.Sprintf("console.log(localStorage.getItem('%s') + 1, sessionStorage.getItem('%s'));", key, key)
		first = local + "1"
		formWhy = fmt.Sprintf("`\"%s\" + 1` — снова конкатенация строки с числом", local)
		if localNull {
			first = "1"
			formWhy = "`null + 1` приводит null к 0 и даёт число 1"
		}
	}
	codeA = append(codeA, "", sc.comment, finalLine)
	answer := first + " " + session
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Сайт открыт во вкладке A, оба хранилища пусты. Выполняется первый фрагмент; %s выполняется второй. "+
			"Что он выведет в консоль (два значения через пробел)?", sc.stem),
		Code:   &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(codeA, "\n")},
		Answer: itembank.Answer{Text: answer},
		Explanation: fmt.Sprintf("Web Storage хранит только строки: `setItem('%s', %d)` сохраняет `\"%d\"`. Далее %s. `localStorage` общий для "+
			"всех вкладок одного origin и переживает перезагрузку и закрытие вкладки, поэтому первое значение — `%s` (%s). "+
			"Второе значение — `%s`: %s. Отсутствующий ключ `getItem` возвращает как `null`, а не `undefined` или пустую строку.",
			key, n, n, opWhy, first, formWhy, session, sc.why),
	}, nil
}

// forcedReflowCount — сколько синхронных пересчётов layout вызовет чередование чтения геометрии и записи стилей.
type forcedReflowCount struct{}

func (forcedReflowCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.rendering_perf.forced-reflow-count.v1", Spec: itembank.SpecFrontend, Topic: "rendering_perf",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.6, B: 0.9, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"dom", "layout", "performance"},
	}
}

func (f forcedReflowCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF603)
	n := itembank.IntBetween(r, 3, 8)
	names := itembank.Pick(r, [][]string{{"boxes", "box"}, {"cards", "card"}, {"rows", "row"}, {"items", "item"}})
	coll, el := names[0], names[1]
	readProp := itembank.Pick(r, []string{"offsetHeight", "getBoundingClientRect().height", "clientHeight", "offsetWidth"})
	dirtyStart := r.IntN(2) == 0
	pattern := r.IntN(5)

	code := []string{fmt.Sprintf("const %s = document.querySelectorAll('.%s');", coll, el)}
	needContainer := dirtyStart || pattern == 4
	if needContainer {
		code = append(code, "const container = document.querySelector('.list');")
	}
	var ops []byte // 'R' — чтение геометрии, 'W' — запись стилей
	if dirtyStart {
		code = append(code, itembank.Pick(r, []string{"container.style.width = '600px';", "container.classList.add('compact');"}))
		ops = append(ops, 'W')
	}
	writeH := fmt.Sprintf("%s.style.height = (h + 10) + 'px';", el)
	writeFixed := itembank.Pick(r, []string{fmt.Sprintf("%s.classList.add('expanded');", el), fmt.Sprintf("%s.style.marginTop = '8px';", el)})
	var patternWhy string
	switch pattern {
	case 0: // чтение → запись в каждой итерации
		code = append(code, fmt.Sprintf("for (const %s of %s) {", el, coll), fmt.Sprintf("  const h = %s.%s;", el, readProp), "  "+writeH, "}")
		for i := 0; i < n; i++ {
			ops = append(ops, 'R', 'W')
		}
		patternWhy = "чтение в каждой итерации, начиная со второй, застаёт стили, изменённые предыдущей итерацией — классический layout thrashing"
	case 1: // запись → чтение в каждой итерации
		code = append(code, fmt.Sprintf("for (const %s of %s) {", el, coll), "  "+writeFixed, fmt.Sprintf("  console.log(%s.%s);", el, readProp), "}")
		for i := 0; i < n; i++ {
			ops = append(ops, 'W', 'R')
		}
		patternWhy = "каждое чтение идёт сразу после записи в той же итерации, поэтому пересчёт нужен на каждом элементе"
	case 2: // сначала все чтения, потом все записи
		code = append(code,
			fmt.Sprintf("const heights = [...%s].map(%s => %s.%s);", coll, el, el, readProp),
			fmt.Sprintf("%s.forEach((%s, i) => {", coll, el),
			fmt.Sprintf("  %s.style.height = (heights[i] + 10) + 'px';", el),
			"});")
		for i := 0; i < n; i++ {
			ops = append(ops, 'R')
		}
		for i := 0; i < n; i++ {
			ops = append(ops, 'W')
		}
		patternWhy = "чтения сгруппированы до записей: после первого пересчёта layout остаётся актуальным, а записи ничего не читают"
	case 3: // чтение → запись → чтение
		code = append(code, fmt.Sprintf("for (const %s of %s) {", el, coll), fmt.Sprintf("  const h = %s.%s;", el, readProp), "  "+writeH,
			fmt.Sprintf("  console.log(%s.%s);", el, readProp), "}")
		for i := 0; i < n; i++ {
			ops = append(ops, 'R', 'W', 'R')
		}
		patternWhy = "второе чтение в итерации идёт сразу после записи и вызывает пересчёт, после чего первое чтение следующей итерации уже видит актуальный layout"
	default: // все записи, затем одно чтение
		code = append(code, fmt.Sprintf("for (const %s of %s) {", el, coll), "  "+writeFixed, "}", fmt.Sprintf("console.log(container.%s);", readProp))
		for i := 0; i < n; i++ {
			ops = append(ops, 'W')
		}
		ops = append(ops, 'R')
		patternWhy = "записи только помечают layout устаревшим, и единственный пересчёт происходит при чтении после цикла"
	}

	count, reads, writes := 0, 0, 0
	dirty := false
	for _, op := range ops {
		if op == 'W' {
			dirty = true
			writes++
			continue
		}
		reads++
		if dirty {
			count++
			dirty = false
		}
	}
	startNote := "перед запуском кода layout актуален"
	if dirtyStart {
		startNote = "перед циклом код меняет стили контейнера"
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("На странице %d элементов `.%s`; %s. Сколько синхронных пересчётов layout (forced reflow) вызовет этот фрагмент? "+
			"Считайте, что любое изменение стилей помечает layout устаревшим, а чтение геометрии при устаревшем layout заставляет браузер "+
			"пересчитать его немедленно.", n, el, startNote),
		fmt.Sprintf("В `%s` — %d элементов; %s. Сколько раз браузер будет вынужден синхронно пересчитать layout при выполнении кода? "+
			"Модель: запись стилей делает layout устаревшим, чтение геометрии при устаревшем layout немедленно пересчитывает его.", coll, n, startNote),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(code, "\n")},
		Answer: itembank.Answer{Number: float64(count), Tolerance: 0},
		Explanation: fmt.Sprintf("Запись стилей сама по себе не вызывает пересчёт — она лишь помечает layout устаревшим, а пересчёт откладывается до "+
			"следующего кадра или до первого чтения геометрии (`%s`). Здесь %s. Всего записей %d, чтений %d, из них при устаревшем layout — %d: "+
			"столько и будет синхронных пересчётов. Ошибка — считать каждую запись или каждое чтение отдельным пересчётом.",
			readProp, patternWhy, writes, reads, count),
	}, nil
}

// useMemoRecompute — сколько раз вызовется тяжёлая функция под useMemo при заданных действиях пользователя
// и перерисовках родителя: пересчёт только в рендерах, где изменилась хотя бы одна зависимость.
type useMemoRecompute struct{}

func (useMemoRecompute) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.rendering_perf.usememo-recompute.v1", Spec: itembank.SpecFrontend, Topic: "rendering_perf",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.4, B: -0.5, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"react", "hooks", "memoization"},
	}
}

func (f useMemoRecompute) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF604)
	comp := itembank.Pick(r, []string{"List", "Table", "Catalog", "Grid"})
	fn := itembank.Pick(r, []string{"filterItems", "sortRows", "buildIndex", "prepareRows"})
	deps := r.IntN(5) // 0 — [items, query], 1 — [items], 2 — [], 3 — [items, query, sortAsc], 4 — без useMemo
	typed := itembank.IntBetween(r, 1, 5)
	toggles := itembank.IntBetween(r, 0, 3)
	parents := itembank.IntBetween(r, 0, 3)
	if toggles == 0 && parents == 0 {
		toggles = itembank.IntBetween(r, 1, 3)
	}
	stableItems := r.IntN(2) == 0

	call := fmt.Sprintf("%s(items, query, sortAsc)", fn)
	var memoLine, depsText string
	switch deps {
	case 0:
		depsText = "[items, query]"
	case 1:
		depsText = "[items]"
	case 2:
		depsText = "[]"
	case 3:
		depsText = "[items, query, sortAsc]"
	}
	if deps == 4 {
		memoLine = fmt.Sprintf("  const visible = %s;", call)
	} else {
		memoLine = fmt.Sprintf("  const visible = useMemo(() => %s, %s);", call, depsText)
	}
	code := []string{
		fmt.Sprintf("function %s({ items }) {", comp),
		"  const [query, setQuery] = useState('');",
		"  const [sortAsc, setSortAsc] = useState(true);",
		memoLine,
		"  return (",
		"    <>",
		"      <input value={query} onChange={e => setQuery(e.target.value)} />",
		"      <button onClick={() => setSortAsc(v => !v)}>Сортировка</button>",
		"      <Rows rows={visible} />",
		"    </>",
		"  );",
		"}",
	}

	onQuery := deps == 0 || deps == 3 || deps == 4
	onToggle := deps == 3 || deps == 4
	onParent := deps == 4 || (!stableItems && deps != 2)
	calls := 1
	var parts []string
	if onQuery {
		calls += typed
	}
	renders := func(k int) string { return ruPlural(k, "рендер", "рендера", "рендеров") }
	parts = append(parts, fmt.Sprintf("ввод: %s, `query` меняется → %s", renders(typed), yesNo(onQuery, typed)))
	if toggles > 0 {
		if onToggle {
			calls += toggles
		}
		parts = append(parts, fmt.Sprintf("сортировка: %s, `sortAsc` меняется → %s", renders(toggles), yesNo(onToggle, toggles)))
	}
	if parents > 0 {
		if onParent {
			calls += parents
		}
		itemsNote := "ссылка `items` та же"
		if !stableItems {
			itemsNote = "каждый раз новый массив `items`"
		}
		parts = append(parts, fmt.Sprintf("перерисовки родителя: %s, %s → %s", renders(parents), itemsNote, yesNo(onParent, parents)))
	}
	totalRenders := 1 + typed + toggles + parents

	var actions []string
	actions = append(actions, fmt.Sprintf("вводит в поле %s (по одному `onChange` на символ)", ruPlural(typed, "символ", "символа", "символов")))
	if toggles > 0 {
		actions = append(actions, fmt.Sprintf("%s нажимает кнопку сортировки", ruPlural(toggles, "раз", "раза", "раз")))
	}
	if parents > 0 {
		itemsDesc := "тот же массив `items` (ссылка не меняется)"
		if !stableItems {
			itemsDesc = "каждый раз новый массив `items` (например, `items={data.filter(isActive)}`)"
		}
		actions = append(actions, fmt.Sprintf("родитель %s перерисовывает `%s`, передавая %s", ruPlural(parents, "раз", "раза", "раз"), comp, itemsDesc))
	}
	var rule string
	switch deps {
	case 2:
		rule = "С пустым массивом зависимостей функция вызывается один раз при монтировании; результат не обновляется при вводе — это баг устаревших данных, но число вызовов равно 1."
	case 4:
		rule = "Без `useMemo` функция вызывается в каждом рендере, включая те, где её аргументы не менялись."
	default:
		rule = fmt.Sprintf("`useMemo` с `%s` вызывает функцию при первом рендере и затем только в рендерах, где хотя бы одна зависимость изменилась по `Object.is`; "+
			"новый массив от родителя — это новая ссылка, а значит изменение `items`.", depsText)
	}
	stem := itembank.Pick(r, []string{
		fmt.Sprintf("Компонент `%s` смонтирован. Затем пользователь %s. Сколько раз всего будет вызвана функция `%s`, включая первый рендер?",
			comp, strings.Join(actions, ", "), fn),
		fmt.Sprintf("После монтирования `%s` происходит следующее: пользователь %s. Сколько вызовов `%s` выполнится за всё время, считая вызов при монтировании?",
			comp, strings.Join(actions, ", "), fn),
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "jsx", Source: strings.Join(code, "\n")},
		Answer: itembank.Answer{Number: float64(calls), Tolerance: 0},
		Explanation: fmt.Sprintf("%s Монтирование — 1 вызов; %s. Итого %d. Всего рендеров %d — ошибка считать их все, как и считать, "+
			"что пересчёт зависит от того, использует ли функция переменную, а не от списка зависимостей.",
			rule, strings.Join(parts, "; "), calls, totalRenders),
	}, nil
}

// yesNo — «пересчитывается: k» / «не пересчитывается» для пояснений useMemo.
func yesNo(recompute bool, k int) string {
	if recompute {
		return fmt.Sprintf("+%d", k)
	}
	return "без пересчёта"
}

// lazyChunksBytes — сколько килобайт или запросов JavaScript уйдёт за сеанс при code splitting с React.lazy:
// статический импорт попадает в основной бандл, общий модуль двух ленивых страниц выделяется в отдельный чанк, чанки кэшируются.
type lazyChunksBytes struct{}

func (lazyChunksBytes) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.bundling.lazy-chunks-bytes.v1", Spec: itembank.SpecFrontend, Topic: "bundling",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.4, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"bundling", "code-splitting", "react"},
	}
}

type lazyPage struct {
	name       string
	size       int
	lazy       bool
	usesShared bool
}

func (f lazyChunksBytes) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF605)
	names := itembank.PickN(r, []string{"Home", "Profile", "Settings", "Reports", "Orders", "Catalog"}, 3)
	shared := itembank.Pick(r, []string{"charts", "editor", "maps", "tables"})
	pages := make([]lazyPage, 3)
	for i := range pages {
		pages[i] = lazyPage{name: names[i], size: itembank.IntBetween(r, 2, 6) * 10, lazy: true}
	}
	if r.IntN(2) == 0 { // одна страница импортирована статически — она в основном бандле
		pages[r.IntN(3)].lazy = false
	}
	for _, i := range itembank.PickN(r, []int{0, 1, 2}, 2) {
		pages[i].usesShared = true
	}
	mainBase := itembank.IntBetween(r, 8, 20) * 10
	sharedSize := itembank.IntBetween(r, 3, 9) * 10

	// Общий модуль живёт отдельным чанком, только если обе его страницы ленивые; иначе он нужен при старте и попадает в main.
	sharedInMain := false
	mainSize := mainBase
	for _, p := range pages {
		if !p.lazy {
			mainSize += p.size
			if p.usesShared {
				sharedInMain = true
			}
		}
	}
	if sharedInMain {
		mainSize += sharedSize
	}

	// Маршрут: стартовая страница и 2–3 перехода без повторов подряд.
	route := []int{0}
	for steps := itembank.IntBetween(r, 2, 3); steps > 0; steps-- {
		next := r.IntN(3)
		for next == route[len(route)-1] {
			next = r.IntN(3)
		}
		route = append(route, next)
	}

	loaded := map[string]bool{}
	total, requests, initial := 0, 0, 0
	var steps []string
	for pos, idx := range route {
		p := pages[idx]
		var got []string
		add := func(name string, size int) {
			if loaded[name] {
				return
			}
			loaded[name] = true
			total += size
			requests++
			got = append(got, fmt.Sprintf("%s %d KB", name, size))
		}
		if pos == 0 {
			add("main", mainSize)
		}
		if p.lazy {
			add(p.name, p.size)
			if p.usesShared && !sharedInMain {
				add(shared, sharedSize)
			}
		}
		if pos == 0 {
			initial = total
		}
		stepName := fmt.Sprintf("переход на %s", p.name)
		if pos == 0 {
			stepName = fmt.Sprintf("старт на %s", p.name)
		}
		if len(got) == 0 {
			steps = append(steps, stepName+": ничего, всё уже в кэше")
		} else {
			steps = append(steps, stepName+": "+strings.Join(got, " + "))
		}
	}

	var code []string
	for _, p := range pages {
		if p.lazy {
			code = append(code, fmt.Sprintf("const %s = lazy(() => import('./pages/%s'));", p.name, p.name))
		} else {
			code = append(code, fmt.Sprintf("import %s from './pages/%s';", p.name, p.name))
		}
	}
	var sharedUsers []string
	for _, p := range pages {
		if p.usesShared {
			sharedUsers = append(sharedUsers, p.name)
		}
	}
	code = append(code, "", fmt.Sprintf("// в %s.jsx и %s.jsx:", sharedUsers[0], sharedUsers[1]), fmt.Sprintf("import { render } from '../lib/%s';", shared))

	var sizes []string
	sizes = append(sizes, fmt.Sprintf("код приложения без страниц и без `%s` — %d KB", shared, mainBase))
	for _, p := range pages {
		sizes = append(sizes, fmt.Sprintf("%s — %d KB", p.name, p.size))
	}
	sizes = append(sizes, fmt.Sprintf("модуль `%s` — %d KB", shared, sharedSize))
	var routeNames []string
	for _, idx := range route[1:] {
		routeNames = append(routeNames, pages[idx].name)
	}
	rules := "Статически импортированный код попадает в основной бандл; модуль, общий для нескольких ленивых чанков, сборщик выделяет в отдельный чанк, " +
		"который загружается вместе с первой нуждающейся в нём страницей; загруженные чанки кэшируются и повторно не запрашиваются."
	var question string
	var answer int
	switch q := r.IntN(10); {
	case q < 6:
		question = "Сколько килобайт JavaScript загрузит браузер за весь сеанс?"
		answer = total
	case q < 8:
		question = "Сколько запросов за JavaScript-чанками выполнит браузер за весь сеанс?"
		answer = requests
	default:
		question = fmt.Sprintf("Сколько килобайт JavaScript загрузится при первом открытии `%s` (до переходов)?", pages[0].name)
		answer = initial
	}
	stem := fmt.Sprintf("Размеры после сборки: %s. %s Пользователь открывает приложение на странице %s, затем переходит: %s. %s",
		strings.Join(sizes, "; "), rules, pages[0].name, strings.Join(routeNames, " → "), question)
	sharedNote := fmt.Sprintf("`%s` импортируют две ленивые страницы, поэтому он лежит в отдельном чанке и загружается один раз — при первой из них.", shared)
	if sharedInMain {
		sharedNote = fmt.Sprintf("Одна из страниц, использующих `%s`, импортирована статически, поэтому модуль нужен при старте и входит в main (%d KB), "+
			"а не в отдельный чанк.", shared, mainSize)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "javascript", Source: strings.Join(code, "\n")},
		Answer: itembank.Answer{Number: float64(answer), Tolerance: 0},
		Explanation: fmt.Sprintf("Основной бандл: %d KB (базовый код%s). %s Шаги: %s. За сеанс — %d KB и %s, при первом открытии — %d KB. "+
			"Ошибки — считать общий модуль в каждой использующей его странице, заново считать чанк при повторном переходе или выделять статически "+
			"импортированную страницу в отдельный чанк.", mainSize, mainExtras(pages, sharedInMain, shared), sharedNote, strings.Join(steps, "; "),
			total, ruPlural(requests, "запрос", "запроса", "запросов"), initial),
	}, nil
}

// mainExtras перечисляет, что добавлено в основной бандл помимо базового кода.
func mainExtras(pages []lazyPage, sharedInMain bool, shared string) string {
	var extras []string
	for _, p := range pages {
		if !p.lazy {
			extras = append(extras, p.name)
		}
	}
	if sharedInMain {
		extras = append(extras, "`"+shared+"`")
	}
	if len(extras) == 0 {
		return ""
	}
	return " + " + strings.Join(extras, " + ")
}

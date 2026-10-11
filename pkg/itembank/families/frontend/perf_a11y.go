package frontend

import (
	"fmt"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(memoRenderCount{})
	itembank.Register(virtualListRows{})
	itembank.Register(accessibleName{})
}

// memoRenderCount — сколько раз отрендерится дочерний компонент с React.memo при
// изменении состояния родителя: зависит от стабильности пропсов.
type memoRenderCount struct{}

func (memoRenderCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.rendering_perf.memo-render-count.v1", Spec: itembank.SpecFrontend, Topic: "rendering_perf",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.4, B: 0.8, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"react", "memo", "performance"},
	}
}

type propVariant struct {
	decl   string // строка объявления в родителе (может быть пустой)
	jsx    string // атрибут в JSX
	stable bool
	once   bool // меняется только при первом символе, дальше стабилен
	why    string
}

func (f memoRenderCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF401)
	child := itembank.Pick(r, []string{"Item", "Row", "Card", "Badge"})
	n := itembank.IntBetween(r, 2, 6)
	useMemo := r.IntN(4) != 0
	callbacks := []propVariant{
		{"", "onSelect={() => setCount(c => c + 1)}", false, false, "стрелочная функция создаётся заново на каждом рендере"},
		{"  const handleSelect = useCallback(() => setCount(c => c + 1), []);", "onSelect={handleSelect}", true, false, "useCallback с `[]` возвращает одну и ту же функцию"},
		{"  const handleSelect = useCallback(() => setCount(c => c + 1), [query]);", "onSelect={handleSelect}", false, false, "useCallback с `[query]` пересоздаёт функцию при каждом символе"},
		{"  const handleSelect = useCallback(() => setCount(c => c + 1), [count]);", "onSelect={handleSelect}", true, false, "useCallback с `[count]` не меняется, пока count неизменен"},
	}
	seconds := []propVariant{
		{"", "label=\"Open\"", true, false, "строковый литерал сравнивается по значению"},
		{"", "label={query}", false, false, "`query` меняется при каждом символе"},
		{"", "label={count}", true, false, "`count` при вводе текста не меняется"},
		{"", "meta={{ id: 1 }}", false, false, "литерал объекта — новая ссылка на каждом рендере"},
		{"", "label={query.length > 0}", false, true, "`query.length > 0` меняется с false на true только при первом символе, дальше стабилен"},
	}
	cb := itembank.Pick(r, callbacks)
	second := itembank.Pick(r, seconds)
	childDecl := fmt.Sprintf("function %s({ onSelect, ...rest }) {\n  console.log('%s render');\n  return <button onClick={onSelect}>{String(rest.label ?? '')}</button>;\n}", child, child)
	if useMemo {
		childDecl = fmt.Sprintf("const %s = memo(function %s({ onSelect, ...rest }) {\n  console.log('%s render');\n  return <button onClick={onSelect}>{String(rest.label ?? '')}</button>;\n});", child, child, child)
	}
	parent := []string{
		"function Search() {",
		"  const [query, setQuery] = useState('');",
		"  const [count, setCount] = useState(0);",
	}
	if cb.decl != "" {
		parent = append(parent, cb.decl)
	}
	parent = append(parent,
		"  return (",
		"    <>",
		"      <input value={query} onChange={e => setQuery(e.target.value)} />",
		fmt.Sprintf("      <%s %s %s />", child, cb.jsx, second.jsx),
		"      <span>{count}</span>",
		"    </>",
		"  );",
		"}")
	renders := 1 + n
	var explain string
	switch {
	case !useMemo:
		explain = fmt.Sprintf("Без `memo` дочерний компонент рендерится вместе с родителем при каждом изменении `query`: "+
			"1 первичный рендер + %d обновлений = %d.", n, renders)
	case cb.stable && second.stable:
		renders = 1
		explain = fmt.Sprintf("`memo` сравнивает пропсы поверхностно: %s, %s — ни один проп не меняется, поэтому %s рендерится "+
			"только один раз, при монтировании.", cb.why, second.why, child)
	case cb.stable && second.once:
		renders = 2
		explain = fmt.Sprintf("`memo` сравнивает пропсы поверхностно: %s, а %s. Поэтому %s рендерится при монтировании и ещё раз "+
			"после первого символа: всего 2.", cb.why, second.why, child)
	default:
		var broken []string
		if !cb.stable {
			broken = append(broken, cb.why)
		}
		if !second.stable {
			broken = append(broken, second.why)
		}
		explain = fmt.Sprintf("`memo` сравнивает пропсы поверхностно, и сравнение проваливается: %s. Значит, каждый из %d рендеров "+
			"родителя тянет рендер %s: 1 + %d = %d.", strings.Join(broken, "; "), n, child, n, renders)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Пользователь печатает в поле ввода %d символов подряд (по одному событию `onChange` на символ), кнопку не нажимает. "+
			"Сколько раз в консоли появится `%s render`, считая первый рендер?", n, child),
		Code:        &itembank.CodeBlock{Lang: "jsx", Source: childDecl + "\n\n" + strings.Join(parent, "\n")},
		Answer:      itembank.Answer{Number: float64(renders), Tolerance: 0},
		Explanation: explain,
	}, nil
}

// virtualListRows — сколько строк отрисует виртуализированный список при заданной прокрутке.
type virtualListRows struct{}

func (virtualListRows) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.rendering_perf.virtual-list-rows.v1", Spec: itembank.SpecFrontend, Topic: "rendering_perf",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.1, B: -0.3, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"virtualization", "performance"},
	}
}

func (f virtualListRows) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF402)
	total := itembank.Pick(r, []int{100, 200, 500, 1000})
	rowH := itembank.Pick(r, []int{20, 24, 32, 40, 48, 50})
	viewport := itembank.Pick(r, []int{300, 400, 480, 500, 600, 720})
	overscan := itembank.Pick(r, []int{0, 1, 2, 3, 5})
	scrollTop := itembank.IntBetween(r, 0, (total*rowH-viewport)/8) * 8
	first := scrollTop/rowH - overscan
	if first < 0 {
		first = 0
	}
	last := (scrollTop+viewport+rowH-1)/rowH - 1 + overscan
	if last > total-1 {
		last = total - 1
	}
	count := last - first + 1
	code := "const first = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);\n" +
		"const last = Math.min(total - 1, Math.ceil((scrollTop + viewportHeight) / rowHeight) - 1 + overscan);\n" +
		"const rows = items.slice(first, last + 1);"
	askFirst := r.IntN(3) == 0
	answer := count
	question := "Сколько элементов попадёт в `rows`?"
	if askFirst {
		answer = first
		question = "Чему будет равен `first` (индекс первой отрисованной строки)?"
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Виртуализированный список: `total = %d`, `rowHeight = %d`, `viewportHeight = %d`, `overscan = %d`, "+
			"текущий `scrollTop = %d`. %s", total, rowH, viewport, overscan, scrollTop, question),
		Code:   &itembank.CodeBlock{Lang: "javascript", Source: code},
		Answer: itembank.Answer{Number: float64(answer), Tolerance: 0},
		Explanation: fmt.Sprintf("first = max(0, ⌊%d/%d⌋ − %d) = %d; last = min(%d, ⌈(%d + %d)/%d⌉ − 1 + %d) = %d. "+
			"Частично видимая строка тоже рендерится (поэтому округление вверх для нижней границы), overscan добавляет запас "+
			"с обеих сторон, но не выходит за границы списка. Строк: %d − %d + 1 = %d.",
			scrollTop, rowH, overscan, first, total-1, scrollTop, viewport, rowH, overscan, last, last, first, count),
	}, nil
}

// accessibleName — какое имя получит кнопка у скринридера по алгоритму accessible name.
type accessibleName struct{}

func (accessibleName) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "frontend.accessibility.accessible-name.v1", Spec: itembank.SpecFrontend, Topic: "accessibility",
		Grade: itembank.GradeJunior, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: -0.4, C: 0.25}, TimeLimit: 90 * time.Second, Tags: []string{"a11y", "aria"},
	}
}

func (f accessibleName) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xF403)
	words := itembank.PickN(r, []string{"Сохранить", "Удалить", "Поиск", "Закрыть", "Отправить", "Настройки", "Фильтр", "Избранное", "Корзина", "Профиль", "Обновить", "Печать"}, 5)
	var hasLabelledBy, hasLabel, hasTitle bool
	var contentKind int // 0 — текст, 1 — img с alt, 2 — svg aria-hidden + текст, 3 — img alt="" + текст, 4 — img alt + текст
	// Нужно не меньше четырёх различимых кандидатов на имя, иначе вариантов ответа не хватит.
	for sources := 0; sources < 4; {
		hasLabelledBy, hasLabel, hasTitle = r.IntN(3) == 0, r.IntN(2) == 0, r.IntN(2) == 0
		contentKind = r.IntN(5)
		sources = 1
		if contentKind == 4 {
			sources = 3 // alt, текст и их сочетание
		}
		for _, b := range []bool{hasLabelledBy, hasLabel, hasTitle} {
			if b {
				sources++
			}
		}
	}
	var attrs []string
	var pre string
	var candidates []string
	if hasLabelledBy {
		pre = fmt.Sprintf("<span id=\"hint\">%s</span>\n", words[0])
		attrs = append(attrs, "aria-labelledby=\"hint\"")
		candidates = append(candidates, words[0])
	}
	if hasLabel {
		attrs = append(attrs, fmt.Sprintf("aria-label=\"%s\"", words[1]))
		candidates = append(candidates, words[1])
	}
	if hasTitle {
		attrs = append(attrs, fmt.Sprintf("title=\"%s\"", words[2]))
		candidates = append(candidates, words[2])
	}
	var content, contentName string
	switch contentKind {
	case 0:
		content, contentName = words[3], words[3]
	case 1:
		content, contentName = fmt.Sprintf("<img src=\"i.svg\" alt=\"%s\">", words[4]), words[4]
	case 2:
		content, contentName = fmt.Sprintf("<svg aria-hidden=\"true\"><use href=\"#i\"/></svg> %s", words[3]), words[3]
	case 3:
		content, contentName = fmt.Sprintf("<img src=\"i.svg\" alt=\"\"> %s", words[3]), words[3]
	default:
		content, contentName = fmt.Sprintf("<img src=\"i.svg\" alt=\"%s\"> %s", words[4], words[3]), words[4]+" "+words[3]
		candidates = append(candidates, words[3], words[4])
	}
	candidates = append(candidates, contentName)
	attrStr := ""
	if len(attrs) > 0 {
		attrStr = " " + strings.Join(itembank.Shuffle(r, attrs), " ")
	}
	html := fmt.Sprintf("%s<button%s>\n  %s\n</button>", pre, attrStr, content)
	var name, rule string
	switch {
	case hasLabelledBy:
		name, rule = words[0], "`aria-labelledby` имеет высший приоритет: имя берётся из текста элемента, на который он ссылается"
	case hasLabel:
		name, rule = words[1], "`aria-label` перекрывает содержимое кнопки и `title`"
	default:
		name, rule = contentName, "без ARIA-атрибутов имя собирается из содержимого: текст и `alt` изображений (`alt=\"\"` и `aria-hidden` пропускаются), `title` — лишь запасной вариант"
	}
	var distractors []string
	seen := map[string]bool{name: true}
	for _, c := range itembank.Shuffle(r, candidates) {
		if !seen[c] && len(distractors) < 3 {
			seen[c] = true
			distractors = append(distractors, c)
		}
	}
	if len(distractors) < 3 {
		distractors = append(distractors, "пустое имя")
	}
	options, key := itembank.ShuffledOptions(r, name, distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    "Какое доступное имя (accessible name) получит кнопка в дереве доступности?",
		Code:    &itembank.CodeBlock{Lang: "html", Source: html},
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: fmt.Sprintf("Порядок вычисления имени: aria-labelledby → aria-label → содержимое (текст, alt) → title. Здесь %s, "+
			"поэтому имя — «%s».", rule, name),
	}, nil
}

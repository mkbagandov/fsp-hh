package common

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(gitBranchCommits{})
	itembank.Register(gitRebaseMerge{})
	itembank.Register(gitResetModes{})
	itembank.Register(gitConflictSteps{})
}

var (
	gitFeatureWords = []string{"login", "search", "export", "cache", "payments", "profile", "reports", "notify", "upload", "i18n", "metrics", "billing"}
	gitCommitMsgs   = []string{"add login form", "fix typo in readme", "update deps", "add search index", "refactor config", "add tests",
		"fix cache ttl", "add csv export", "bump version", "add logging", "fix null check", "add retry"}
	gitFileNames = []string{"config.yaml", "main.go", "README.md", "schema.sql", "index.ts", "Dockerfile", "handler.py", "styles.css",
		"Makefile", "app.js", "models.py", "routes.go"}
)

// gitRepo — минимальная модель графа коммитов: ответы вычисляются по достижимости, а не прописываются.
type gitRepo struct {
	parents  [][]int // parents[i] — родители коммита i
	names    []string
	branches map[string]int // ветка → индекс коммита-вершины
}

func newGitRepo() *gitRepo { return &gitRepo{branches: map[string]int{}} }

func (g *gitRepo) addCommit(name string, parents ...int) int {
	g.parents = append(g.parents, parents)
	g.names = append(g.names, name)
	return len(g.names) - 1
}

// commit создаёт коммит на вершине ветки (ветка создаётся, если её ещё нет).
func (g *gitRepo) commit(branch, name string) int {
	var parents []int
	if head, ok := g.branches[branch]; ok {
		parents = []int{head}
	}
	id := g.addCommit(name, parents...)
	g.branches[branch] = id
	return id
}

// reachable — множество коммитов, достижимых из head по родителям.
func (g *gitRepo) reachable(head int) map[int]bool {
	seen := map[int]bool{}
	stack := []int{head}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[c] {
			continue
		}
		seen[c] = true
		stack = append(stack, g.parents[c]...)
	}
	return seen
}

func (g *gitRepo) count(branch string) int { return len(g.reachable(g.branches[branch])) }

// merge — `git merge from` на ветке into. merged — создан коммит слияния, ff — был fast-forward;
// оба false означают «Already up to date».
func (g *gitRepo) merge(into, from string, noFF bool) (merged, ff bool) {
	h, f := g.branches[into], g.branches[from]
	if g.reachable(h)[f] {
		return false, false
	}
	if g.reachable(f)[h] && !noFF {
		g.branches[into] = f
		return false, true
	}
	g.branches[into] = g.addCommit("M", h, f)
	return true, false
}

// rebase — `git rebase onto` на ветке branch: переносит собственные коммиты branch поверх
// вершины onto. Возвращает число переписанных коммитов.
func (g *gitRepo) rebase(branch, onto string) int {
	base := g.branches[onto]
	inBranch := g.reachable(g.branches[branch])
	if inBranch[base] {
		return 0 // ветка уже содержит onto — нечего переносить
	}
	inOnto := g.reachable(base)
	var own []int
	for c := range inBranch {
		if !inOnto[c] {
			own = append(own, c)
		}
	}
	sort.Ints(own) // линейная история: порядок создания совпадает с топологическим
	if len(own) == 0 {
		g.branches[branch] = base // ветка отстаёт — fast-forward
		return 0
	}
	tip := base
	for _, c := range own {
		tip = g.addCommit(g.names[c]+"'", tip)
	}
	g.branches[branch] = tip
	return len(own)
}

// gitBranchCommits — какие коммиты входят в историю ветки после серии checkout/commit/merge.
type gitBranchCommits struct{}

func (gitBranchCommits) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.git.branch-commits.v1", Spec: itembank.SpecCommon, Topic: "git",
		Grade: itembank.GradeMiddle, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.2, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"git"},
	}
}

func (f gitBranchCommits) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE1A)
	feature := "feature/" + itembank.Pick(r, gitFeatureWords)
	msgs := itembank.PickN(r, gitCommitMsgs, 7) // максимум 2 базовых + 3 на ветке + 2 на main
	for attempt := 0; attempt < 50; attempt++ {
		repo := newGitRepo()
		var cmds, labels []string
		var ids []int
		commit := func(branch string) {
			label := fmt.Sprintf("C%d", len(labels)+1)
			ids = append(ids, repo.commit(branch, label))
			cmds = append(cmds, fmt.Sprintf("git commit -m \"%s: %s\"", label, msgs[len(labels)]))
			labels = append(labels, label)
		}
		for i, base := 0, itembank.IntBetween(r, 1, 2); i < base; i++ {
			commit("main")
		}
		cmds = append(cmds, "git checkout -b "+feature)
		current := feature
		var seq []string
		for i, k := 0, itembank.IntBetween(r, 1, 3); i < k; i++ {
			seq = append(seq, feature)
		}
		for i, k := 0, itembank.IntBetween(r, 0, 2); i < k; i++ {
			seq = append(seq, "main")
		}
		for _, b := range itembank.Shuffle(r, seq) {
			if b != current {
				cmds = append(cmds, "git checkout "+b)
				current = b
			}
			commit(b)
		}
		doMerge := r.IntN(3) != 0
		merged, ff := false, false
		into, from := "main", feature
		if doMerge {
			if r.IntN(2) == 0 {
				into, from = feature, "main"
			}
			if current != into {
				cmds = append(cmds, "git checkout "+into)
			}
			merged, ff = repo.merge(into, from, false)
			cmds = append(cmds, "git merge "+from)
		}
		asked := "main"
		if r.IntN(2) == 0 {
			asked = feature
		}
		reach := repo.reachable(repo.branches[asked])
		var correct, wrong []string
		for i, label := range labels {
			text := fmt.Sprintf("%s — %s", label, msgs[i])
			if reach[ids[i]] {
				correct = append(correct, text)
			} else {
				wrong = append(wrong, text)
			}
		}
		if doMerge {
			text := "M — коммит слияния"
			if merged && reach[len(repo.names)-1] {
				correct = append(correct, text)
			} else {
				wrong = append(wrong, text)
			}
		}
		if len(correct) < 2 || len(wrong) < 1 || len(correct)+len(wrong) > 6 {
			continue
		}
		mergeNote := ""
		if doMerge {
			switch {
			case ff:
				mergeNote = fmt.Sprintf("Слияние `%s` в `%s` прошло как fast-forward: у `%s` не было собственных новых коммитов, указатель просто "+
					"передвинулся, коммит слияния не создавался. ", from, into, into)
			case merged:
				mergeNote = fmt.Sprintf("Слияние `%s` в `%s` создало коммит M с двумя родителями: вся история `%s` вошла в `%s`, а сама ветка `%s` "+
					"не изменилась. ", from, into, from, into, from)
			default:
				mergeNote = fmt.Sprintf("`git merge %s` на ветке `%s` ничего не изменил: все коммиты `%s` уже входили в её историю (Already up to date). ",
					from, into, from)
			}
		}
		options, keys := itembank.ShuffledMulti(r, correct, wrong...)
		return itembank.Instance{
			FamilyID: f.Meta().ID, Seed: seed,
			Stem: fmt.Sprintf("В пустом репозитории выполнили команды из листинга (каждый `git commit` создаёт коммит с указанным именем). "+
				"Какие из перечисленных коммитов входят в историю ветки `%s`, то есть будут показаны командой `git log %s`? Выберите все подходящие.",
				asked, asked),
			Code:    &itembank.CodeBlock{Lang: "bash", Source: strings.Join(cmds, "\n")},
			Options: options,
			Answer:  itembank.Answer{Keys: keys},
			Explanation: fmt.Sprintf("%sВ историю ветки входят коммиты, достижимые от её вершины по цепочке родителей. Для `%s` это: %s. Не входят: %s. "+
				"Типичная ошибка — считать, что более поздние по времени коммиты попадают во все ветки, или что слияние меняет историю ветки-источника.",
				mergeNote, asked, strings.Join(correct, "; "), strings.Join(wrong, "; ")),
		}, nil
	}
	return itembank.Instance{}, errors.New("gitBranchCommits: не удалось построить сценарий")
}

// gitRebaseMerge — число коммитов в истории после rebase и/или merge (fast-forward, --no-ff, переписывание).
type gitRebaseMerge struct{}

func (gitRebaseMerge) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.git.rebase-vs-merge.v1", Spec: itembank.SpecCommon, Topic: "git",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: 0.8, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"git"},
	}
}

func (f gitRebaseMerge) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE1B)
	feature := "feature/" + itembank.Pick(r, gitFeatureWords)
	base := itembank.IntBetween(r, 1, 3)
	m := itembank.IntBetween(r, 1, 4)
	k := itembank.IntBetween(r, 0, 3)
	repo := newGitRepo()
	for i := 0; i < base; i++ {
		repo.commit("main", fmt.Sprintf("B%d", i+1))
	}
	repo.branches[feature] = repo.branches["main"]
	for i := 0; i < m; i++ {
		repo.commit(feature, fmt.Sprintf("F%d", i+1))
	}
	for i := 0; i < k; i++ {
		repo.commit("main", fmt.Sprintf("M%d", i+1))
	}
	countQuestion := func(branch string) string {
		return fmt.Sprintf("Сколько коммитов выведет `git rev-list --count %s` после выполнения команд?", branch)
	}
	rebaseNote := func() string {
		if k == 0 {
			return fmt.Sprintf("`main` не ушла вперёд, поэтому rebase ничего не переписывает — `%s` уже основана на вершине `main`.", feature)
		}
		return fmt.Sprintf("rebase переносит %d %s `%s` поверх вершины `main`, создавая их копии с новыми хешами; коммит слияния не появляется.",
			m, pluralRu(m, "коммит", "коммита", "коммитов"), feature)
	}
	sum := func(withMerge bool) string {
		s := fmt.Sprintf("%d базовых + %d на main + %d на ветке", base, k, m)
		if withMerge {
			s += " + 1 коммит слияния"
		}
		return s
	}

	var cmds []string
	var question, explain string
	var answer int
	switch r.IntN(7) {
	case 0:
		cmds = []string{"git checkout " + feature, "git rebase main"}
		repo.rebase(feature, "main")
		answer = repo.count(feature)
		question = countQuestion(feature)
		explain = fmt.Sprintf("%s История `%s`: %s = %d.", rebaseNote(), feature, sum(false), answer)
	case 1:
		cmds = []string{"git checkout main", "git merge " + feature}
		_, ff := repo.merge("main", feature, false)
		answer = repo.count("main")
		question = countQuestion("main")
		if ff {
			explain = fmt.Sprintf("`main` не имела новых коммитов, поэтому слияние выполнено fast-forward: указатель `main` передвинут на вершину `%s`, "+
				"коммит слияния не создан: %s = %d.", feature, sum(false), answer)
		} else {
			explain = fmt.Sprintf("Ветки расходятся, поэтому создаётся коммит слияния с двумя родителями: %s = %d.", sum(true), answer)
		}
	case 2:
		cmds = []string{"git checkout main", "git merge --no-ff " + feature}
		repo.merge("main", feature, true)
		answer = repo.count("main")
		question = countQuestion("main")
		explain = fmt.Sprintf("Флаг --no-ff запрещает fast-forward: коммит слияния создаётся, даже если `main` не ушла вперёд: %s = %d.", sum(true), answer)
	case 3:
		cmds = []string{"git checkout " + feature, "git rebase main", "git checkout main", "git merge " + feature}
		repo.rebase(feature, "main")
		repo.merge("main", feature, false)
		answer = repo.count("main")
		question = countQuestion("main")
		explain = fmt.Sprintf("%s После rebase `%s` содержит всю историю `main`, поэтому merge — fast-forward без коммита слияния: %s = %d.",
			rebaseNote(), feature, sum(false), answer)
	case 4:
		cmds = []string{"git checkout " + feature, "git rebase main"}
		answer = repo.rebase(feature, "main")
		question = fmt.Sprintf("Сколько коммитов ветки `%s` будут переписаны (получат новые хеши) в результате выполнения команд?", feature)
		explain = rebaseNote() + fmt.Sprintf(" Базовые коммиты и коммиты `main` не меняются; переписано %d.", answer)
	case 5:
		cmds = []string{"git checkout " + feature, "git merge main"}
		merged, _ := repo.merge(feature, "main", false)
		answer = repo.count(feature)
		question = countQuestion(feature)
		if merged {
			explain = fmt.Sprintf("Ветки расходятся, поэтому на `%s` создаётся коммит слияния с двумя родителями: %s = %d.", feature, sum(true), answer)
		} else {
			explain = fmt.Sprintf("Вершина `main` уже входит в историю `%s`: merge сообщает Already up to date, история не меняется: %s = %d.",
				feature, sum(false), answer)
		}
	default:
		cmds = []string{"git checkout main", "git merge " + feature}
		merged, _ := repo.merge("main", feature, false)
		for c := range repo.reachable(repo.branches["main"]) {
			if len(repo.parents[c]) == 2 {
				answer++
			}
		}
		question = "Сколько коммитов слияния (с двумя родителями) окажется в истории `main` после выполнения команд?"
		if merged {
			explain = "Ветки расходятся — создаётся ровно один коммит слияния с двумя родителями."
		} else {
			explain = "`main` не ушла вперёд — слияние выполняется fast-forward, коммит слияния не создаётся."
		}
	}
	mainNote := "на `main` за это время новых коммитов не появилось"
	if k > 0 {
		mainNote = fmt.Sprintf("параллельно на `main` добавлено ещё %d %s", k, pluralRu(k, "коммит", "коммита", "коммитов"))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("На ветке `main` сделано %d %s. От её вершины создана ветка `%s`, на которой затем сделано %d %s; %s. "+
			"После этого выполнены команды из листинга. %s",
			base, pluralRu(base, "коммит", "коммита", "коммитов"), feature, m, pluralRu(m, "коммит", "коммита", "коммитов"), mainNote, question),
		Code:   &itembank.CodeBlock{Lang: "bash", Source: strings.Join(cmds, "\n")},
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: explain + " Типичная ошибка — считать, что merge всегда создаёт коммит слияния, а rebase добавляет коммиты, " +
			"не убирая старые из истории ветки.",
	}, nil
}

// gitResetModes — эффект git reset --soft / --mixed / --hard на HEAD, индекс и рабочую директорию.
type gitResetModes struct{}

func (gitResetModes) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.git.reset-modes.v1", Spec: itembank.SpecCommon, Topic: "git",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.2, B: 0.3, C: 0.25}, TimeLimit: 120 * time.Second, Tags: []string{"git"},
	}
}

func (f gitResetModes) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE1C)
	branch := itembank.Pick(r, []string{"main", "develop", "feature/" + itembank.Pick(r, gitFeatureWords)})
	n := itembank.IntBetween(r, 3, 5)
	back := itembank.IntBetween(r, 1, 2)
	target := n - back
	files := itembank.PickN(r, gitFileNames, 2)
	staged, modified := files[0], files[1]
	undone := fmt.Sprintf("коммита C%d", n)
	if back == 2 {
		undone = fmt.Sprintf("коммитов C%d и C%d", n-1, n)
	}
	flags := []string{"--soft ", "--mixed ", "", "--hard "}
	mode := r.IntN(len(flags))
	cmd := fmt.Sprintf("git reset %sHEAD~%d", flags[mode], back)

	soft := fmt.Sprintf("HEAD переходит на C%d; изменения %s и файла `%s` остаются в индексе (staged); `%s` остаётся изменённым в рабочей директории",
		target, undone, staged, modified)
	mixed := fmt.Sprintf("HEAD переходит на C%d; изменения %s и файла `%s` сняты с индекса, но сохранены в рабочей директории; `%s` остаётся изменённым",
		target, undone, staged, modified)
	hard := fmt.Sprintf("HEAD переходит на C%d; изменения %s, файла `%s` и файла `%s` отброшены — индекс и рабочая директория приведены к C%d",
		target, undone, staged, modified, target)
	revert := fmt.Sprintf("HEAD остаётся на C%d; создаётся новый коммит, отменяющий изменения %s; индекс и файлы `%s`, `%s` не затрагиваются",
		n, undone, staged, modified)
	all := []string{soft, mixed, mixed, hard}
	correct := all[mode]
	var distractors []string
	for _, t := range []string{soft, mixed, hard, revert} {
		if t != correct {
			distractors = append(distractors, t)
		}
	}
	modeNames := []string{"--soft", "--mixed", "--mixed (режим по умолчанию)", "--hard"}
	options, key := itembank.ShuffledOptions(r, correct, distractors...)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("В репозитории на ветке `%s` коммиты C1 → … → C%d, HEAD указывает на C%d. Файл `%s` изменён и добавлен в индекс (`git add`), "+
			"файл `%s` изменён, но не добавлен. Выполняется команда `%s`. Что произойдёт?", branch, n, n, staged, modified, cmd),
		Options: options,
		Answer:  itembank.Answer{Keys: []string{key}},
		Explanation: fmt.Sprintf("`--soft` двигает только HEAD (и ветку), `--mixed` — режим по умолчанию — HEAD и индекс, `--hard` — HEAD, индекс и рабочую "+
			"директорию. HEAD~%d — это C%d, режим здесь %s, поэтому: %s. Новый коммит reset никогда не создаёт — это делает `git revert`. "+
			"Типичная ошибка — ожидать, что --mixed теряет изменения, или что --soft убирает их из индекса.", back, target, modeNames[mode], correct),
	}, nil
}

// gitConflictSteps — порядок действий при разрешении конфликта слияния или rebase.
type gitConflictSteps struct{}

func (gitConflictSteps) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "common.git.conflict-steps.v1", Spec: itembank.SpecCommon, Topic: "git",
		Grade: itembank.GradeJunior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.0, B: -0.5, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"git"},
	}
}

func (f gitConflictSteps) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xC0DE1D)
	cur := "feature/" + itembank.Pick(r, gitFeatureWords)
	target := itembank.Pick(r, []string{"main", "develop", "master"})
	file := itembank.Pick(r, gitFileNames)
	edit := fmt.Sprintf("Открыть `%s`, оставить нужный вариант кода и удалить маркеры `<<<<<<<`, `=======`, `>>>>>>>`", file)
	add := fmt.Sprintf("`git add %s` — пометить конфликт в файле разрешённым", file)
	var steps []string
	var goal, finish string
	switch r.IntN(3) {
	case 0:
		goal = fmt.Sprintf("влить изменения ветки `%s`", target)
		finish = "`git commit` завершает слияние, создавая коммит слияния"
		steps = []string{
			fmt.Sprintf("`git merge %s` — Git сообщает о конфликте в `%s`", target, file),
			edit, add,
			"`git commit` — завершить слияние, создав коммит слияния",
			fmt.Sprintf("`git push origin %s` — отправить результат в удалённый репозиторий", cur),
		}
	case 1:
		goal = fmt.Sprintf("подтянуть изменения `%s` из удалённого репозитория", target)
		finish = "`git commit` завершает слияние, создавая коммит слияния"
		steps = []string{
			fmt.Sprintf("`git pull origin %s` — при слиянии возникает конфликт в `%s`", target, file),
			edit, add,
			"`git commit` — завершить слияние, создав коммит слияния",
			fmt.Sprintf("`git push origin %s` — отправить результат в удалённый репозиторий", cur),
		}
	default:
		goal = fmt.Sprintf("перебазировать свою ветку на вершину `%s`", target)
		finish = "`git rebase --continue` продолжает перенос оставшихся коммитов"
		steps = []string{
			fmt.Sprintf("`git rebase %s` — Git останавливается на конфликте в `%s`", target, file),
			edit, add,
			"`git rebase --continue` — продолжить перенос оставшихся коммитов",
			fmt.Sprintf("`git push --force-with-lease origin %s` — обновить переписанную ветку на сервере", cur),
		}
	}
	if r.IntN(2) == 0 {
		steps = steps[:4]
	}
	options, order := itembank.OrderingOptions(r, steps)
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem: fmt.Sprintf("Разработчик работает на ветке `%s` и хочет %s; в файле `%s` возникает конфликт. Расположите шаги в правильном порядке.",
			cur, goal, file),
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: fmt.Sprintf("Сначала выполняется команда, которая приводит к конфликту; затем файл правится вручную и маркеры удаляются; `git add` "+
			"сообщает Git, что конфликт разрешён; после этого %s; и только потом изменения отправляются на сервер (после rebase — с --force-with-lease, "+
			"так как история переписана). Типичная ошибка — выполнять commit или --continue до `git add` или делать push до завершения слияния.", finish),
	}, nil
}

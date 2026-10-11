// Package devops — семейства заданий для DevOps / SRE: Linux, Docker, Kubernetes,
// CI/CD, наблюдаемость, сети, инфраструктура как код, инциденты.
package devops

import (
	"fmt"
	"math"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(dockerLayerCache{})
	itembank.Register(rollingUpdatePods{})
}

// dockerLayerCache — какие слои пересоберутся после изменения файла.
type dockerLayerCache struct{}

func (dockerLayerCache) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.docker.layer-cache.v1", Spec: itembank.SpecDevOps, Topic: "docker",
		Grade: itembank.GradeMiddle, Format: itembank.FormatSingleChoice,
		IRT: itembank.IRTParams{A: 1.3, B: 0.0, C: 0.25}, TimeLimit: 120 * time.Second, Tags: []string{"docker"},
	}
}

type stackVariant struct {
	base     string
	depFiles string
	depCmd   string
	buildCmd string
	srcFile  string
}

var stackVariants = []stackVariant{
	{"golang:1.25", "go.mod go.sum", "go mod download", "go build -o /app ./cmd/api", "internal/service/handler.go"},
	{"node:22", "package.json package-lock.json", "npm ci", "npm run build", "src/components/App.tsx"},
	{"python:3.12", "requirements.txt", "pip install -r requirements.txt", "python -m compileall .", "app/routes.py"},
}

func (f dockerLayerCache) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xD0C001)
	v := itembank.Pick(r, stackVariants)
	service := itembank.Ident(r, "service")
	dockerfile := fmt.Sprintf("FROM %s\nWORKDIR /src/%s\nCOPY %s ./\nRUN %s\nCOPY . .\nRUN %s\nCMD [\"/app\"]", v.base, service, v.depFiles, v.depCmd, v.buildCmd)
	changed := itembank.Pick(r, []string{"src", "deps", "none"})
	var changedDesc, correct, explain string
	fromDeps := "начиная с `COPY " + v.depFiles + " ./` (установка зависимостей и всё ниже)"
	fromSrc := "начиная с `COPY . .` (копирование исходников и сборка)"
	onlyBuild := "только `RUN " + v.buildCmd + "`"
	all := "все инструкции, кэш сбрасывается целиком"
	switch changed {
	case "src":
		changedDesc = fmt.Sprintf("изменили исходный файл `%s`", v.srcFile)
		correct = fromSrc
		explain = "Кэш слоёв инвалидируется с первой инструкции, чей вход изменился: `COPY . .` копирует изменённый файл, поэтому эта инструкция и все последующие выполняются заново, а слой с зависимостями берётся из кэша."
	case "deps":
		changedDesc = fmt.Sprintf("изменили файл зависимостей `%s`", strings.Split(v.depFiles, " ")[0])
		correct = fromDeps
		explain = "Файл зависимостей копируется раньше исходников специально: его изменение инвалидирует слой `COPY " + v.depFiles + "` и всё ниже, включая установку зависимостей и сборку."
	default:
		changedDesc = "ничего не меняли, пересобрали тем же контекстом"
		correct = "ни одна: все слои берутся из кэша"
		explain = "Без изменений входов каждой инструкции все слои берутся из кэша; выполняется только сравнение контрольных сумм."
	}
	options, key := itembank.ShuffledOptions(r, correct, fromDeps, fromSrc, onlyBuild, all, "ни одна: все слои берутся из кэша")
	// ShuffledOptions убирает дубликат верного; оставляем 4 варианта.
	if len(options) > 4 {
		trimmed := make([]itembank.Option, 0, 4)
		kept := 0
		for _, o := range options {
			if o.Key == key || kept < 3 {
				if o.Key != key {
					kept++
				}
				trimmed = append(trimmed, o)
			}
		}
		options = trimmed
		for i := range options {
			options[i].Key = []string{"a", "b", "c", "d"}[i]
			if options[i].Text == correct {
				key = options[i].Key
			}
		}
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        fmt.Sprintf("Образ собирали по Dockerfile ниже, затем %s и запустили сборку снова. Какие инструкции выполнятся заново?", changedDesc),
		Code:        &itembank.CodeBlock{Lang: "dockerfile", Source: dockerfile},
		Options:     options,
		Answer:      itembank.Answer{Keys: []string{key}},
		Explanation: explain + " Типичная ошибка — считать, что пересобирается только изменившийся слой: кэш работает «сверху вниз до первого отличия».",
	}, nil
}

// rollingUpdatePods — число подов при rolling update.
type rollingUpdatePods struct{}

func (rollingUpdatePods) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.kubernetes.rolling-update-pods.v1", Spec: itembank.SpecDevOps, Topic: "kubernetes",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.2, B: 0.2, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"kubernetes"},
	}
}

func (f rollingUpdatePods) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xD0C002)
	replicas := itembank.IntBetween(r, 3, 12)
	app := itembank.Ident(r, "service")
	var surgeStr, unavailStr string
	var surge, unavail int
	if r.IntN(2) == 0 {
		pct := itembank.Pick(r, []int{10, 20, 25, 30, 50})
		surgeStr = fmt.Sprintf("%d%%", pct)
		surge = int(math.Ceil(float64(replicas) * float64(pct) / 100))
	} else {
		surge = itembank.IntBetween(r, 1, 3)
		surgeStr = fmt.Sprintf("%d", surge)
	}
	if r.IntN(2) == 0 {
		pct := itembank.Pick(r, []int{10, 20, 25, 30, 50})
		unavailStr = fmt.Sprintf("%d%%", pct)
		unavail = int(math.Floor(float64(replicas) * float64(pct) / 100))
	} else {
		unavail = itembank.IntBetween(r, 0, 2)
		unavailStr = fmt.Sprintf("%d", unavail)
	}
	manifest := fmt.Sprintf("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: %s\nspec:\n  replicas: %d\n  strategy:\n    type: RollingUpdate\n    rollingUpdate:\n      maxSurge: %s\n      maxUnavailable: %s",
		app, replicas, surgeStr, unavailStr)
	askMax := r.IntN(2) == 0
	var answer int
	var question, explain string
	if askMax {
		answer = replicas + surge
		question = "Какое максимальное число подов Deployment может существовать одновременно во время обновления?"
		explain = fmt.Sprintf("maxSurge %s от %d реплик — это %d (проценты округляются вверх): максимум %d + %d = %d подов.", surgeStr, replicas, surge, replicas, surge, answer)
	} else {
		answer = replicas - unavail
		question = "Какое минимальное число подов гарантированно доступно во время обновления?"
		explain = fmt.Sprintf("maxUnavailable %s от %d реплик — это %d (проценты округляются вниз): минимум %d − %d = %d доступных подов.", unavailStr, replicas, unavail, replicas, unavail, answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        question,
		Code:        &itembank.CodeBlock{Lang: "yaml", Source: manifest},
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: explain + " Типичная ошибка — округлять проценты в другую сторону или складывать surge с unavailable.",
	}, nil
}

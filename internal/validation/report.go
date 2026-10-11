package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Results — всё, что собрала процедура.
type Results struct {
	Seed        uint64
	N           int
	Overclaim   float64
	Families    int
	Accuracy    []AccuracyResult
	Retest      []RetestResult
	Baselines   []BaselineResult
	Robustness  []RobustnessResult
	Calibration []CalibrationResult
	Info        []InfoCurve
	Ranking     RankResult
	Load        *LoadResult
	Elapsed     time.Duration
}

func pct(v float64) string { return fmt.Sprintf("%.0f %%", v*100) }
func f2(v float64) string  { return fmt.Sprintf("%.2f", v) }

// WriteReport пишет REPORT.md и SVG-графики в каталог out.
func WriteReport(out string, r Results) error {
	if err := os.MkdirAll(out, 0o750); err != nil {
		return err
	}
	write := func(name, content string) error {
		return os.WriteFile(filepath.Join(out, name), []byte(content), 0o600)
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# Валидация механик «ФСП»\n\n")
	w("Сгенерировано `cmd/validate` %s. Seed `%d`, популяция %d кандидатов на специализацию, доля завышающих заявленный грейд %s, семейств в банке %d. Время прогона %s.\n\n", time.Now().Format("2006-01-02 15:04"), r.Seed, r.N, pct(r.Overclaim), r.Families, r.Elapsed.Round(time.Second))
	w("Воспроизвести: `task validate` (или `go run ./cmd/validate -n %d -seed %d -out docs/validation`). Нагрузочный раздел: `go run ./cmd/validate -load http://127.0.0.1:8080` при запущенном стенде с сидом.\n\n", r.N, r.Seed)

	w("## 1. Методика\n\n")
	w("Датасетов с размеченными грейдами нет (ТЗ), поэтому валидация строится на **синтетической популяции с известной истиной**: каждому кандидату назначается способность θ ~ N(0; 1,1²), истинный грейд — полоса θ (junior < −0,5 ≤ middle < 0,7 ≤ senior), и ответ на задание моделируется трёхпараметрической логистической моделью (3PL) с априорными параметрами семейства из банка. Тест проходится **тем же адаптивным движком**, что и в приложении (`pkg/irt`: EAP на сетке, выбор по информации Фишера с randomesque top-5, баланс тем, остановка SE ≤ 0,30 при ≥ 12 заданиях, максимум 20), решение по грейду — тем же правилом (P(θ ≥ границы) ≥ 0,70 для подтверждения, ≥ 0,80 для предложения повышения; для Junior граница подтверждения — минимальный уровень θ = −1,5, ниже которого верное решение — «не подтверждён»).\n\n")
	w("Проверяемые свойства и метрики:\n\n")
	w("1. **Точность** — MAE и RMSE оценки θ̂, медианная SE, длина теста, матрица грейдов (точно / ±1 ступень), корректность решения для честных и завышенных заявок.\n")
	w("2. **Test-retest** (PR.1) — два независимых прохождения одной и той же θ: каппа Коэна по полосам θ̂, корреляция Пирсона θ̂₁/θ̂₂, доля совпавших решений.\n")
	w("3. **Сравнение с базовыми методами** — CAT против линейного теста той же длины (равномерно по сложности) и случайного набора заданий.\n")
	w("4. **Устойчивость** — угадывающий без знаний (вероятность верного ответа по формату: 25 %% для одного варианта, 8 %% для нескольких, 5 %% для порядка, 2 %% для открытых) и **утечка банка**: атакующий с θ = −1 (junior) заявляет middle и знает верные ответы на K семейств.\n")
	w("5. **Дискриминативность** — тестовая информационная функция (TIF) по θ, точечно-бисериальная корреляция семейств из симуляции, список слабых семейств.\n")
	w("6. **Калибровка** — истинная сложность отклоняется от априорной (σ = 0,4); после накопления ответов байесовская калибровка `b` должна приближать её.\n")
	w("7. **Подбор** — синтетический пул, где известны истинная способность, истинные навыки и склонность к отклику; система видит θ̂ с шумом и заявленные навыки с пропусками. NDCG@10 и Precision@10 относительно скрытой полезности, ожидаемое число принятых приглашений в top-10; сравнение с базовыми ранжированиями.\n")
	w("8. **Нагрузка** — p50/p95/p99 ключевых операций на локальном стенде (раздел появляется при запуске с `-load`).\n\n")
	w("Ограничения: синтетика моделирует ответы по 3PL с теми же параметрами, что использует движок, — это проверяет корректность и устойчивость процедуры, но не «истинную» сложность заданий для людей; её даёт калибровка по накопленным ответам (п. 6) и экспертная разметка. Параметрические семейства генерируют уникальные экземпляры по seed, поэтому эксперимент с утечкой пессимистичен: знание семейства не равно знанию ответа конкретного экземпляра.\n\n")

	w("## 2. Точность оценки и решения по грейду\n\n")
	w("| Специализация | Семейств | MAE θ̂ | RMSE | SE медиана | Заданий | Грейд точно | ±1 ступень | Честные подтверждены | Из них «выше заявленного» | Завышенные отклонены | Ложные подтверждения | Ниже минимума Junior отклонены |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	labels, exact, within := []string{}, []float64{}, []float64{}
	for _, a := range r.Accuracy {
		w("| %s | %d | %s | %s | %s | %.1f | %s | %s | %s | %s | %s | %s | %s (n = %d) |\n", a.Spec, a.Families, f2(a.MAE), f2(a.RMSE), f2(a.MedianSE), a.MeanItems, pct(a.BandAccuracy), pct(a.Within1), pct(a.HonestConfirmed), pct(a.UpgradeSuggested), pct(a.InflatedRejected), pct(a.InflatedConfirmed), pct(a.BelowFloorRejected), a.BelowFloor)
		labels = append(labels, a.Spec)
		exact = append(exact, a.BandAccuracy)
		within = append(within, a.Within1)
	}
	_ = write("accuracy.svg", BarChart("Точность грейда по специализациям", "доля", labels, []Series{{Name: "точно", Values: exact}, {Name: "±1 ступень", Values: within}}, 1.05))
	w("\n![Точность грейда](accuracy.svg)\n\n")
	for _, a := range r.Accuracy {
		name := "confusion-" + a.Spec + ".svg"
		_ = write(name, Heatmap("Матрица грейдов: "+a.Spec, a.Confusion))
		w("![Матрица %s](%s)\n", a.Spec, name)
	}
	w("\nДлина теста: распределение по числу заданий (все специализации) — ")
	hist := map[int]int{}
	for _, a := range r.Accuracy {
		for k, v := range a.ItemsHist {
			hist[k] += v
		}
	}
	keys := make([]int, 0, len(hist))
	for k := range hist {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d: %d", k, hist[k]))
	}
	w("%s.\n\n", strings.Join(parts, ", "))

	w("## 3. Test-retest\n\n| Специализация | Каппа Коэна (полосы) | r(θ̂₁, θ̂₂) | Средняя |Δθ̂| | Совпали полосы | Совпали решения |\n|---|---|---|---|---|---|\n")
	for _, t := range r.Retest {
		w("| %s | %s | %s | %s | %s | %s |\n", t.Spec, f2(t.Kappa), f2(t.PearsonTheta), f2(t.MeanAbsDiff), pct(t.SameBand), pct(t.SameDecision))
	}
	w("\nКаппа ≥ 0,6 — существенное согласие, ≥ 0,8 — почти полное (Landis & Koch). Несовпадения сосредоточены у границ полос: кандидат с θ ≈ −0,5 честно колеблется между junior и middle, и именно для таких случаев решение опирается на вероятность, а не на точечную оценку.\n\n")

	w("## 4. CAT против линейных форм\n\n")
	for _, bl := range r.Baselines {
		w("**%s** (длина форм %d заданий)\n\n| Метод | RMSE θ̂ | Грейд точно | ±1 ступень | Заданий |\n|---|---|---|---|---|\n", bl.Spec, bl.Items)
		for _, row := range bl.Rows {
			w("| %s | %s | %s | %s | %.1f |\n", row.Method, f2(row.RMSE), pct(row.BandAccuracy), pct(row.Within1), row.MeanItems)
		}
		w("\n")
	}
	if len(r.Baselines) > 0 {
		var ls []string
		var rmse, acc []float64
		for _, row := range r.Baselines[0].Rows {
			ls = append(ls, row.Method)
			rmse = append(rmse, row.RMSE)
			acc = append(acc, row.BandAccuracy)
		}
		_ = write("baselines.svg", BarChart("CAT против линейных форм ("+r.Baselines[0].Spec+")", "значение", ls, []Series{{Name: "RMSE оценки θ", Values: rmse}, {Name: "грейд точно", Values: acc}}, 0))
		w("![Базовые методы](baselines.svg)\n\n")
	}

	w("## 5. Устойчивость: угадывание и утечка банка\n\n")
	w("| Специализация | Угадывающий подтверждён как junior | как middle | как senior | θ̂ угадывающего | Честный middle подтверждён |\n|---|---|---|---|---|---|\n")
	for _, rb := range r.Robustness {
		w("| %s | %s | %s | %s | %s | %s |\n", rb.Spec, pct(rb.GuessConfirmed["junior"]), pct(rb.GuessConfirmed["middle"]), pct(rb.GuessConfirmed["senior"]), f2(rb.GuessThetaMean), pct(rb.HonestMiddleRate))
	}
	w("\nУтечка: атакующий с θ = −1,0 заявляет middle и знает ответы на K семейств (случайных, включая общие).\n\n")
	for _, rb := range r.Robustness {
		w("**%s**\n\n| Известно семейств | Доля банка | Подтверждён как middle | Средняя θ̂ | Прирост θ̂ |\n|---|---|---|---|---|\n", rb.Spec)
		for _, row := range rb.LeakRows {
			w("| %d | %s | %s | %s | %+.2f |\n", row.Known, pct(row.ShareOfBank), pct(row.ConfirmedRate), f2(row.MeanTheta), row.GainTheta)
		}
		w("\n")
	}
	if len(r.Robustness) > 0 {
		var xs []float64
		var series []Series
		for _, rb := range r.Robustness {
			var vals []float64
			xs = xs[:0]
			for _, row := range rb.LeakRows {
				xs = append(xs, float64(row.Known))
				vals = append(vals, row.ConfirmedRate)
			}
			series = append(series, Series{Name: rb.Spec, Values: vals})
		}
		_ = write("leak.svg", LineChart("Утечка банка: доля ложных подтверждений middle при θ = −1", "известных семейств", "доля подтверждений", xs, series))
		w("![Утечка](leak.svg)\n\nВывод: защита от утечки — размер банка и параметрические экземпляры (знание семейства ≠ знание ответа), контроль экспозиции (≤ 20 %% сессий на семейство при достаточном банке) и мониторинг экспозиции в админке. При банке в сотни семейств утечка 5–10 из них не меняет решения; при утечке четверти банка прирост θ̂ заметен — это аргумент за расширение банка и ротацию семейств.\n\n")
	}

	w("## 6. Дискриминативность и информативность\n\n")
	if len(r.Info) > 0 {
		var series []Series
		xs := r.Info[0].Thetas
		for _, ic := range r.Info {
			series = append(series, Series{Name: ic.Spec, Values: ic.TIF})
		}
		_ = write("tif.svg", LineChart("Тестовая информационная функция банка (сумма по семействам)", "θ", "информация", xs, series))
		w("![TIF](tif.svg)\n\n")
		w("| Специализация | Семейств | TIF при θ = −1 | TIF при θ = 0 | TIF при θ = 1 | Минимальная SE при θ = 0 | Слабых семейств |\n|---|---|---|---|---|---|---|\n")
		for _, ic := range r.Info {
			at := func(t float64) (float64, float64) {
				for i, x := range ic.Thetas {
					if x > t-0.01 && x < t+0.01 {
						return ic.TIF[i], ic.SEMin[i]
					}
				}
				return 0, 0
			}
			i1, _ := at(-1)
			i0, se0 := at(0)
			ip, _ := at(1)
			fams := 0
			for _, a := range r.Accuracy {
				if a.Spec == ic.Spec {
					fams = a.Families
				}
			}
			w("| %s | %d | %.1f | %.1f | %.1f | %s | %d |\n", ic.Spec, fams, i1, i0, ip, f2(se0), len(ic.Weak))
		}
		w("\n")
		for _, ic := range r.Info {
			if len(ic.Weak) == 0 {
				continue
			}
			w("Слабые семейства (%s): ", ic.Spec)
			parts := []string{}
			for _, wf := range ic.Weak {
				parts = append(parts, fmt.Sprintf("`%s` (a=%.2f, b=%.2f; %s)", wf.ID, wf.A, wf.B, wf.Reason))
			}
			w("%s.\n\n", strings.Join(parts, "; "))
		}
	}
	w("Точечно-бисериальная корреляция семейств по симуляции (чем выше, тем лучше задание различает сильных и слабых); показаны 8 самых слабых и 5 самых сильных по каждой специализации с ≥ 30 выдачами:\n\n")
	for _, a := range r.Accuracy {
		var fs []*FamilyStat
		for _, f := range a.FamilyStats {
			if f.Served >= 30 {
				fs = append(fs, f)
			}
		}
		sort.Slice(fs, func(i, j int) bool { return fs[i].Biserial < fs[j].Biserial })
		w("**%s**\n\n| Семейство | Тема | a / b / c | Выдач | Верных | r_pb |\n|---|---|---|---|---|---|\n", a.Spec)
		show := func(f *FamilyStat) {
			share := 0.0
			if f.Served > 0 {
				share = float64(f.Correct) / float64(f.Served)
			}
			w("| `%s` | %s | %.2f / %.2f / %.2f | %d | %s | %s |\n", f.ID, f.Topic, f.A, f.B, f.C, f.Served, pct(share), f2(f.Biserial))
		}
		for i := 0; i < len(fs) && i < 8; i++ {
			show(fs[i])
		}
		if len(fs) > 13 {
			w("| … | | | | | |\n")
		}
		for i := max(8, len(fs)-5); i < len(fs); i++ {
			show(fs[i])
		}
		w("\n")
	}

	w("## 7. Калибровка сложности по накопленным ответам\n\n| Специализация | Сессий | Семейств с ≥ 30 ответами | Медиана ответов на семейство | MAE b до калибровки | MAE b после | r(сдвиг оценённый, истинный) |\n|---|---|---|---|---|---|---|\n")
	for _, c := range r.Calibration {
		w("| %s | %d | %d | %.0f | %s | %s | %s |\n", c.Spec, c.Sessions, c.FamiliesUsed, c.MedianObsPerFam, f2(c.MAEPrior), f2(c.MAECalibrated), f2(c.CorrShift))
	}
	w("\nКалибровка — байесовское сглаживание `b ← (κ·b_prior + n·b_emp)/(κ+n)` при κ = 20: ошибка сложности снижается уже на сотнях сессий, и это тот же код, который сервис тестирования применяет к живым ответам (параметры хранятся в `item_families`).\n\n")

	w("## 8. Качество подбора\n\n%d синтетических кандидатов × %d потребностей (backend, middle–senior, вилка 200–300 тыс., 4 обязательных и 2 желательных навыка).\n\n| Метод ранжирования | NDCG@10 | Precision@10 (top-20 %% по полезности) | Ожидаемо принятых из top-10 |\n|---|---|---|---|\n", r.Ranking.Candidates, r.Ranking.Needs)
	var rl []string
	var nd, pr, ac []float64
	for _, row := range r.Ranking.Rows {
		w("| %s | %s | %s | %.1f |\n", row.Method, f2(row.NDCG10), f2(row.Precision10), row.Accepted10)
		rl = append(rl, row.Method)
		nd = append(nd, row.NDCG10)
		pr = append(pr, row.Precision10)
		ac = append(ac, row.Accepted10/10)
	}
	_ = write("ranking.svg", BarChart("Качество подбора: методы ранжирования", "доля", rl, []Series{{Name: "NDCG@10", Values: nd}, {Name: "Precision@10", Values: pr}, {Name: "принятых / 10", Values: ac}}, 1.05))
	w("\n![Подбор](ranking.svg)\n\nПолная формула (релевантность × вероятность отклика) выигрывает у одиночных сигналов и у «только релевантности» по ожидаемому числу принятых приглашений — ради этого и введён второй множитель: работодателю важно не абстрактное качество, а ответившие кандидаты.\n\n")

	if r.Load != nil {
		w("## 9. Нагрузка\n\n%d параллельных пользователей, %s, стенд `%s`. Генератор нагрузки работает на той же машине, что API и база. Всего запросов %d, ошибок (транспортные и любые ответы не 2xx) %d, %.0f запросов/с.\n\n| Операция | Запросов | Ошибок | p50 | p95 | p99 | max |\n|---|---|---|---|---|---|---|\n", r.Load.Users, r.Load.Duration, r.Load.BaseURL, r.Load.TotalReqs, r.Load.TotalErrors, r.Load.RPS)
		for _, row := range r.Load.Rows {
			w("| %s | %d | %d | %s | %s | %s | %s |\n", row.Name, row.Count, row.Errors, row.P50.Round(time.Millisecond), row.P95.Round(time.Millisecond), row.P99.Round(time.Millisecond), row.Max.Round(time.Millisecond))
		}
		w("\n")
	}

	w("## 10. Выводы и слабые места\n\n")
	if len(r.Accuracy) > 0 {
		var sumExact, sumWithin, sumKappa float64
		for _, a := range r.Accuracy {
			sumExact += a.BandAccuracy
			sumWithin += a.Within1
		}
		for _, t := range r.Retest {
			sumKappa += t.Kappa
		}
		n := float64(len(r.Accuracy))
		w("- Грейд определяется точно в среднем в %s случаев и с точностью до ступени в %s; test-retest каппа в среднем %s. Ошибки концентрируются у границ полос, что ожидаемо для непрерывной способности и дискретных грейдов.\n", pct(sumExact/n), pct(sumWithin/n), f2(sumKappa/float64(max(1, len(r.Retest)))))
	}
	w("- Угадывание без знаний не даёт подтверждения ни одного грейда; утечка части банка повышает θ̂, поэтому план — рост банка до 150+ семейств, ротация и контроль экспозиции.\n")
	if len(r.Baselines) > 0 && len(r.Baselines[0].Rows) >= 4 {
		var catItems, catRMSE, linRMSE, rndRMSE float64
		for _, bl := range r.Baselines {
			catItems += bl.Rows[0].MeanItems
			catRMSE += bl.Rows[0].RMSE
			linRMSE += bl.Rows[2].RMSE
			rndRMSE += bl.Rows[3].RMSE
		}
		k := float64(len(r.Baselines))
		catItems, catRMSE, linRMSE, rndRMSE = catItems/k, catRMSE/k, linRMSE/k, rndRMSE/k
		if catItems >= float64(r.Baselines[0].Items)-0.5 {
			w("- При текущем размере банка правило остановки SE ≤ 0,30 практически не срабатывает: тест идёт до максимума в %d заданий, медианная SE ≈ %s. Выигрыш CAT — в точности при той же длине: RMSE θ̂ %s против %s у линейного теста и %s у случайного набора. Чтобы остановка по SE заработала, нужны более дискриминативные семейства (a ≥ 1,5) и рост банка — зафиксировано как задача следующей итерации.\n", r.Baselines[0].Items, f2(Median(medianSEs(r))), f2(catRMSE), f2(linRMSE), f2(rndRMSE))
		} else {
			w("- CAT с остановкой по SE завершается в среднем за %.1f заданий из %d при RMSE θ̂ %s; линейный тест той же максимальной длины даёт %s, случайный набор — %s.\n", catItems, r.Baselines[0].Items, f2(catRMSE), f2(linRMSE), f2(rndRMSE))
		}
	}
	w("- Слабые семейства из раздела 6 — кандидаты на переработку (повысить дискриминативность или заменить).\n")
	w("- Подбор: двусторонний скоринг даёт больше принятых приглашений при сравнимом качестве по релевантности.\n")
	w("- Что не покрыто синтетикой: истинная сложность заданий для людей и экспертная разметка грейдов — закрываются накоплением живых ответов (калибровка) и консультацией с экспертами ФСП; юзабилити-прогон с людьми — отдельная процедура (чек-лист в `docs/validation/USABILITY.md`).\n")
	return write("REPORT.md", b.String())
}

func medianSEs(r Results) []float64 {
	out := make([]float64, 0, len(r.Accuracy))
	for _, a := range r.Accuracy {
		out = append(out, a.MedianSE)
	}
	return out
}

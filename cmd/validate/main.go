// Command validate — воспроизводимая процедура валидации механик тестирования и подбора.
//
//	go run ./cmd/validate                      — полный прогон, отчёт и графики в docs/validation
//	go run ./cmd/validate -quick               — короткая сводка в консоль без файлов
//	go run ./cmd/validate -load http://127.0.0.1:8080   — плюс нагрузочный раздел по запущенному стенду
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"fsp/internal/validation"
	"fsp/pkg/itembank"
	_ "fsp/pkg/itembank/families/all"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "validate:", err)
		os.Exit(1)
	}
}

func run() error {
	quick := flag.Bool("quick", false, "только сводка в консоль (без отчёта и графиков)")
	n := flag.Int("n", 400, "число синтетических кандидатов на специализацию")
	seed := flag.Uint64("seed", 42, "seed симуляции")
	overclaim := flag.Float64("overclaim", 0.3, "доля кандидатов, завышающих грейд на ступень")
	out := flag.String("out", "docs/validation", "каталог для REPORT.md и SVG")
	loadURL := flag.String("load", "", "базовый URL стенда для нагрузочного прогона (пусто — пропустить)")
	loadUsers := flag.Int("load-users", 50, "параллельных пользователей в нагрузочном прогоне")
	loadSeconds := flag.Int("load-seconds", 15, "длительность нагрузочного прогона, с")
	loadEmail := flag.String("load-employer", "demo-employer@seed.fsp.local", "e-mail работодателя для операций подбора в нагрузке")
	loadPassword := flag.String("load-password", "Demo1234!", "пароль работодателя")
	flag.Parse()

	started := time.Now()
	pools := validation.PoolsFromBank(8)
	if len(pools) == 0 {
		return fmt.Errorf("в банке нет специализаций с ≥ 8 семействами (всего %d)", itembank.Count())
	}
	res := validation.Results{Seed: *seed, N: *n, Overclaim: *overclaim, Families: itembank.Count()}
	fmt.Printf("Банк: %d семейств, специализаций: %d, кандидатов на специализацию: %d\n", itembank.Count(), len(pools), *n)
	for _, p := range pools {
		rng := rand.New(rand.NewPCG(*seed, 1))
		people := validation.Population(rng, *n, *overclaim)
		acc := validation.Accuracy(p, people, *seed)
		res.Accuracy = append(res.Accuracy, acc)
		fmt.Printf("%-10s семейств=%-3d | MAE=%.2f RMSE=%.2f SE=%.2f заданий=%.1f | грейд точно %.0f%% ±1 %.0f%% | честные подтверждены %.0f%%, завышенные отклонены %.0f%%\n",
			p.Spec, acc.Families, acc.MAE, acc.RMSE, acc.MedianSE, acc.MeanItems, 100*acc.BandAccuracy, 100*acc.Within1, 100*acc.HonestConfirmed, 100*acc.InflatedRejected)
		if *quick {
			continue
		}
		rt := validation.Retest(p, people[:min(len(people), 300)], *seed)
		res.Retest = append(res.Retest, rt)
		fmt.Printf("           test-retest: каппа=%.2f r=%.2f |Δθ̂|=%.2f совпали решения %.0f%%\n", rt.Kappa, rt.PearsonTheta, rt.MeanAbsDiff, 100*rt.SameDecision)
		res.Baselines = append(res.Baselines, validation.Baselines(p, people[:min(len(people), 300)], *seed, 20))
		rb := validation.Robustness(p, *seed, 200)
		res.Robustness = append(res.Robustness, rb)
		fmt.Printf("           угадывающий подтверждён как middle: %.0f%%; утечка 10 семейств → подтверждён %.0f%%\n", 100*rb.GuessConfirmed["middle"], 100*rb.LeakRows[2].ConfirmedRate)
		cal := validation.Calibration(p, *seed, 400)
		res.Calibration = append(res.Calibration, cal)
		fmt.Printf("           калибровка b: MAE %.2f → %.2f (семейств %d)\n", cal.MAEPrior, cal.MAECalibrated, cal.FamiliesUsed)
		res.Info = append(res.Info, validation.Information(p))
	}
	if *quick {
		return nil
	}
	res.Ranking = validation.Ranking(*seed, 200, 30)
	for _, row := range res.Ranking.Rows {
		fmt.Printf("подбор %-42s NDCG@10=%.2f P@10=%.2f принятых/10=%.1f\n", row.Method, row.NDCG10, row.Precision10, row.Accepted10)
	}
	if *loadURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*loadSeconds+60)*time.Second)
		defer cancel()
		lr, err := validation.Load(ctx, *loadURL, *loadUsers, time.Duration(*loadSeconds)*time.Second, *loadEmail, *loadPassword)
		if err != nil {
			return fmt.Errorf("нагрузка: %w", err)
		}
		res.Load = &lr
		fmt.Printf("нагрузка: %d запросов, %d ошибок, %.0f rps\n", lr.TotalReqs, lr.TotalErrors, lr.RPS)
		for _, row := range lr.Rows {
			fmt.Printf("  %-40s p50=%s p95=%s p99=%s\n", row.Name, row.P50.Round(time.Millisecond), row.P95.Round(time.Millisecond), row.P99.Round(time.Millisecond))
		}
	}
	res.Elapsed = time.Since(started)
	if err := validation.WriteReport(*out, res); err != nil {
		return err
	}
	fmt.Printf("Отчёт: %s/REPORT.md (%s)\n", *out, res.Elapsed.Round(time.Second))
	return nil
}

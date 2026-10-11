package testing

import (
	"context"
	"math"

	"fsp/internal/storage/postgres/sqlcgen"
	"fsp/pkg/irt"
)

// Калибровка сложности по живым ответам (docs/04-testing.md, раздел 7). Параметры те же, что в
// валидации (docs/validation/REPORT.md, раздел 7): там на синтетике с известной истиной ошибка
// сложности снижается с 0,22–0,25 до 0,09–0,14.
const (
	// CalibrationKappa — вес априорной сложности в «ответах»: при 20 ответах эмпирика и априор равноправны.
	CalibrationKappa = 20.0
	// CalibrationMinObs — меньше ответов на семейство — сложность не трогаем.
	CalibrationMinObs = 30
)

// CalibrationReport — итог пересчёта.
type CalibrationReport struct {
	Families int `json:"families" doc:"Семейств с достаточным числом ответов (не меньше 30)"`
	Updated  int `json:"updated" doc:"Семейств, у которых изменилась сложность"`
	Answers  int `json:"answers" doc:"Ответов завершённых сессий, вошедших в расчёт"`
}

type familyPrior struct {
	ID          string
	A, BPrior   float64
	C, BCurrent float64
}

type calibrationUpdate struct {
	ID string
	B  float64
}

// calibrate — расчётная часть без базы: новая сложность для каждого семейства с достаточным числом ответов.
// Расчёт всегда идёт от априорной сложности, поэтому повторный запуск не накапливает сдвиг.
func calibrate(fams []familyPrior, obs map[string][]irt.Observation) (updates []calibrationUpdate, families int) {
	for _, f := range fams {
		o := obs[f.ID]
		if len(o) < CalibrationMinObs {
			continue
		}
		families++
		b := irt.CalibrateDifficulty(irt.Params{A: f.A, B: f.BPrior, C: f.C}, o, CalibrationKappa, CalibrationMinObs)
		b = math.Round(b*1000) / 1000
		if math.Abs(b-f.BCurrent) >= 0.001 {
			updates = append(updates, calibrationUpdate{ID: f.ID, B: b})
		}
	}
	return updates, families
}

// Recalibrate уточняет сложность семейств по ответам завершённых сессий; адаптивный тест сразу
// использует новые значения (параметры читаются из item_families). Дискриминативность a не
// пересчитывается: её контролирует точечно-бисериальная корреляция в админке (слабые семейства
// выключаются вручную). ids ограничивает пересчёт набором семейств; nil — весь банк.
func (s *Service) Recalibrate(ctx context.Context, ids []string) (CalibrationReport, error) {
	var only map[string]bool
	if ids != nil {
		only = make(map[string]bool, len(ids))
		for _, id := range ids {
			only[id] = true
		}
	}
	rows, err := s.q.ListCalibrationObservations(ctx)
	if err != nil {
		return CalibrationReport{}, err
	}
	var rep CalibrationReport
	obs := map[string][]irt.Observation{}
	for _, r := range rows {
		if only != nil && !only[r.FamilyID] {
			continue
		}
		obs[r.FamilyID] = append(obs[r.FamilyID], irt.Observation{Theta: r.Theta, Correct: r.Correct})
		rep.Answers++
	}
	fams, err := s.q.ListItemFamilies(ctx)
	if err != nil {
		return CalibrationReport{}, err
	}
	priors := make([]familyPrior, 0, len(fams))
	current := map[string]float64{}
	for _, f := range fams {
		if only != nil && !only[f.ID] {
			continue
		}
		priors = append(priors, familyPrior{ID: f.ID, A: f.A, BPrior: f.BPrior, C: f.C, BCurrent: f.B})
		current[f.ID] = f.A
	}
	updates, families := calibrate(priors, obs)
	rep.Families = families
	for _, u := range updates {
		if err := s.q.UpdateFamilyParams(ctx, sqlcgen.UpdateFamilyParamsParams{ID: u.ID, A: current[u.ID], B: u.B}); err != nil {
			return rep, err
		}
		rep.Updated++
	}
	s.log.Info("item bank recalibrated", "families", rep.Families, "updated", rep.Updated, "answers", rep.Answers)
	return rep, nil
}

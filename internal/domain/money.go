package domain

// MaxSalary — верхняя граница здравого смысла для вилки (рубли в месяц).
const MaxSalary int64 = 10_000_000

// SalaryRange — зарплатная вилка «от–до» в рублях. Обязательна для вакансий и приглашений.
type SalaryRange struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

// Validate проверяет инвариант вилки: 0 < From ≤ To ≤ MaxSalary.
func (r SalaryRange) Validate() error {
	switch {
	case r.From <= 0:
		return Validation("salary.from", "salary_from_required", "Укажите нижнюю границу вилки в рублях")
	case r.To <= 0:
		return Validation("salary.to", "salary_to_required", "Укажите верхнюю границу вилки в рублях")
	case r.From > r.To:
		return Validation("salary", "salary_range_invalid", "Нижняя граница вилки не может быть больше верхней")
	case r.To > MaxSalary:
		return Validation("salary.to", "salary_too_large", "Верхняя граница вилки выглядит нереалистично")
	}
	return nil
}

// Contains — лежит ли значение внутри вилки.
func (r SalaryRange) Contains(v int64) bool { return v >= r.From && v <= r.To }

// Covers — покрывает ли вилка ожидание кандидата: ожидание не выше верхней границы.
// Кандидат, ожидающий меньше нижней границы, тоже покрыт — ему предложат не меньше From.
func (r SalaryRange) Covers(expected int64) bool { return expected > 0 && expected <= r.To }

// FitScore — компонент соответствия по зарплате для ранжирования:
// 1.0 — ожидание внутри вилки или ниже; 0.5 — выше верхней границы не более чем на 15 %;
// 0 — выше; 0.7 — ожидание не указано (неизвестность, но не отказ).
func (r SalaryRange) FitScore(expected int64) float64 {
	switch {
	case expected <= 0:
		return 0.7
	case expected <= r.To:
		return 1.0
	case float64(expected) <= float64(r.To)*1.15:
		return 0.5
	default:
		return 0
	}
}

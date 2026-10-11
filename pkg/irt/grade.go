package irt

// Band — полоса грейда на шкале θ.
type Band struct {
	Code  string
	Lower float64 // нижняя граница; для самой нижней полосы — −∞ (например −4)
	// MinConfirm — минимальный θ для подтверждения грейда, если он выше Lower (0 — порог = Lower).
	// Полосы классифицируют оценку, а порог подтверждения нижнего грейда отсекает тех, кто ниже
	// минимального уровня: иначе «Junior подтверждён» получал бы и угадывающий.
	MinConfirm float64
}

// confirmAt — порог подтверждения грейда.
func (b Band) confirmAt() float64 {
	if b.MinConfirm != 0 && b.MinConfirm > b.Lower {
		return b.MinConfirm
	}
	return b.Lower
}

// Decision — решение по грейду.
type Decision struct {
	Status      string  // confirmed | not_confirmed | upgrade_suggested
	Claimed     string  // заявленный грейд
	Result      string  // грейд, который считается подтверждённым (пусто при not_confirmed)
	PConfirm    float64 // P(θ ≥ нижней границы заявленного)
	PUpgrade    float64 // P(θ ≥ нижней границы следующего), 0 если следующего нет
	ConfirmMinP float64
	UpgradeMinP float64
}

// Decide применяет вероятностное правило подтверждения (docs/04-testing.md, раздел 6).
// bands — полосы по возрастанию; confirmMinP (0.70) и upgradeMinP (0.80) — пороги.
func Decide(post Posterior, claimed string, bands []Band, confirmMinP, upgradeMinP float64) Decision {
	d := Decision{Status: "not_confirmed", Claimed: claimed, ConfirmMinP: confirmMinP, UpgradeMinP: upgradeMinP}
	idx := -1
	for i, b := range bands {
		if b.Code == claimed {
			idx = i
		}
	}
	if idx < 0 {
		return d
	}
	d.PConfirm = post.ProbAtLeast(bands[idx].confirmAt())
	if idx+1 < len(bands) {
		d.PUpgrade = post.ProbAtLeast(bands[idx+1].confirmAt())
	}
	switch {
	case idx+1 < len(bands) && d.PUpgrade >= upgradeMinP:
		d.Status = "upgrade_suggested"
		d.Result = claimed
	case d.PConfirm >= confirmMinP:
		d.Status = "confirmed"
		d.Result = claimed
	default:
		d.Status = "not_confirmed"
	}
	return d
}

// BandOf возвращает код полосы, в которую попадает точечная оценка θ.
func BandOf(theta float64, bands []Band) string {
	code := ""
	for _, b := range bands {
		if theta >= b.Lower {
			code = b.Code
		}
	}
	return code
}

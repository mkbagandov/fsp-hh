package security

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(caesarXor{})
	itembank.Register(modexpDH{})
	itembank.Register(tokenTTL{})
	itembank.Register(authFlowOrder{})
}

// caesarXor — ручное применение шифра Цезаря или побайтового XOR к коротким данным.
type caesarXor struct{}

func (caesarXor) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.crypto_basics.caesar-xor.v1", Spec: itembank.SpecSecurity, Topic: "crypto_basics",
		Grade: itembank.GradeJunior, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.4, B: -0.9, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"cipher", "xor"},
	}
}

var cipherWords = []string{"SERVER", "ROUTER", "SOCKET", "KERNEL", "CIPHER", "PACKET", "BUFFER", "PROXY", "VAULT", "BADGE", "TOKEN", "AUDIT", "PATCH", "LOGIN", "QUERY", "INDEX", "CACHE", "SHELL", "DIGEST", "NONCE"}

// caesarShift сдвигает заглавные латинские буквы на k позиций вперёд (k может быть отрицательным).
func caesarShift(word string, k int) string {
	var sb strings.Builder
	for _, c := range word {
		sb.WriteByte(byte('A' + ((int(c-'A')+k)%26+26)%26))
	}
	return sb.String()
}

// randBytes возвращает n байт, первый из которых ≥ 0x10 (hex-запись без ведущего нуля).
func randBytes(r interface{ IntN(int) int }, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(r.IntN(256) & 0xFF)
	}
	out[0] = byte((0x10 + r.IntN(0xF0)) & 0xFF)
	return out
}

// hexCount склоняет «N hex-знаков» для N = 4 или 6.
func hexCount(n int) string {
	if n == 4 {
		return "4 hex-знака"
	}
	return fmt.Sprintf("%d hex-знаков", n)
}

func xorBytes(a, b []byte) []byte {
	out := make([]byte, len(a))
	for i := range a {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func xorTrace(a, b []byte) string {
	parts := make([]string, len(a))
	for i := range a {
		parts[i] = fmt.Sprintf("%02x ⊕ %02x = %08b ⊕ %08b = %08b = %02x", a[i], b[i], a[i], b[i], a[i]^b[i], a[i]^b[i])
	}
	return strings.Join(parts, "; ")
}

func (f caesarXor) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC005)
	var stem, answer, explain string
	switch r.IntN(4) {
	case 0, 1:
		word := itembank.Pick(r, cipherWords)
		k := itembank.IntBetween(r, 1, 25)
		ct := caesarShift(word, k)
		pairs := make([]string, len(word))
		for i := range word {
			pairs[i] = fmt.Sprintf("%c→%c", word[i], ct[i])
		}
		if r.IntN(2) == 0 {
			answer = ct
			stem = itembank.Pick(r, []string{
				fmt.Sprintf("Шифр Цезаря сдвигает каждую латинскую букву на k позиций вперёд по алфавиту (после Z снова идёт A). Зашифруйте слово `%s` со сдвигом k = %d. Ответ — слово заглавными буквами.", word, k),
				fmt.Sprintf("Примените к слову `%s` шифр Цезаря со сдвигом %d вперёд (A→%c). Запишите результат заглавными латинскими буквами.", word, k, caesarShift("A", k)[0]),
			})
			explain = fmt.Sprintf("Каждая буква заменяется на стоящую через %d позиций вперёд, с переходом через Z: %s. Итог — %s. "+
				"Типичные ошибки — сдвиг назад вместо вперёд и ошибка на единицу при переходе через конец алфавита.", k, strings.Join(pairs, ", "), ct)
		} else {
			answer = word
			stem = itembank.Pick(r, []string{
				fmt.Sprintf("Слово `%s` получено шифром Цезаря со сдвигом %d вперёд по латинскому алфавиту. Восстановите исходное слово (заглавными буквами).", ct, k),
				fmt.Sprintf("Расшифруйте `%s`: шифр Цезаря, каждая буква была сдвинута на %d позиций вперёд (с переходом Z→A). Ответ заглавными латинскими буквами.", ct, k),
			})
			explain = fmt.Sprintf("Для расшифровки каждую букву сдвигают на %d позиций назад (или на %d вперёд): %s. Исходное слово — %s. "+
				"Типичная ошибка — сдвинуть ещё раз вперёд или перепутать направление при переходе через начало алфавита.", k, 26-k, strings.Join(pairs, ", "), word)
		}
	case 2:
		n := itembank.IntBetween(r, 2, 3)
		a, b := randBytes(r, n), randBytes(r, n)
		for a[0]^b[0] < 0x10 {
			b = randBytes(r, n)
		}
		res := xorBytes(a, b)
		answer = fmt.Sprintf("%x", res)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Вычислите побайтовый XOR значений `%x` и `%x`. Ответ — %s в нижнем регистре без префикса 0x.", a, b, hexCount(2*n)),
			fmt.Sprintf("Два блока по %d байта заданы в hex: `%x` и `%x`. Чему равен их XOR? Запишите %s в нижнем регистре (без 0x).", n, a, b, hexCount(2*n)),
		})
		explain = fmt.Sprintf("XOR выполняется побитово для каждой пары байт: %s. Итог — %s. "+
			"Типичные ошибки — складывать байты как числа или XOR-ить десятичные записи вместо двоичных.", xorTrace(a, b), answer)
	default:
		n := itembank.IntBetween(r, 2, 3)
		m, key := randBytes(r, n), randBytes(r, n)
		for m[0]^key[0] < 0x10 {
			key = randBytes(r, n)
		}
		ct := xorBytes(m, key)
		answer = fmt.Sprintf("%x", m)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Сообщение зашифровано одноразовым блокнотом: c = m ⊕ k (побайтовый XOR). Ключ k = `%x`, шифртекст c = `%x`. Найдите m. Ответ — %s в нижнем регистре без 0x.", key, ct, hexCount(2*n)),
			fmt.Sprintf("Шифр Вернама: открытый текст m, ключ k = `%x`, шифртекст c = m XOR k = `%x`. Восстановите m в hex (нижний регистр, %s, без префикса).", key, ct, hexCount(2*n)),
		})
		explain = fmt.Sprintf("XOR обратим тем же ключом: m = c ⊕ k. Побайтово: %s. Итог — %s. "+
			"Типичная ошибка — пытаться «вычитать» ключ арифметически; XOR — не сложение.", xorTrace(ct, key), answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Text: answer},
		Explanation: explain,
	}, nil
}

// modexpDH — модульное возведение в степень на маленьких числах: Диффи — Хеллман и учебный RSA.
type modexpDH struct{}

func (modexpDH) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.crypto_basics.modexp-dh.v1", Spec: itembank.SpecSecurity, Topic: "crypto_basics",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.2, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"dh", "rsa", "modular"},
	}
}

func modPow(base, exp, mod int) int {
	result := 1
	base %= mod
	for exp > 0 {
		if exp&1 == 1 {
			result = result * base % mod
		}
		base = base * base % mod
		exp >>= 1
	}
	return result
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// modInverse возвращает x ∈ [1, m) с a·x ≡ 1 (mod m); предполагается gcd(a, m) = 1.
func modInverse(a, m int) int {
	t, newT := 0, 1
	rr, newR := m, a%m
	for newR != 0 {
		q := rr / newR
		t, newT = newT, t-q*newT
		rr, newR = newR, rr-q*newR
	}
	if t < 0 {
		t += m
	}
	return t
}

// powTrace описывает вычисление base^exp mod m методом повторного возведения в квадрат.
func powTrace(base, exp, mod int) string {
	var squares []string
	var usedPow []int
	var usedVal []int
	cur := base % mod
	p := 1
	for e := exp; e > 0; e >>= 1 {
		squares = append(squares, fmt.Sprintf("%d^%d ≡ %d", base, p, cur))
		if e&1 == 1 {
			usedPow = append(usedPow, p)
			usedVal = append(usedVal, cur)
		}
		cur = cur * cur % mod
		p *= 2
	}
	pows := make([]string, len(usedPow))
	vals := make([]string, len(usedVal))
	for i := range usedPow {
		pows[i] = fmt.Sprint(usedPow[i])
		vals[i] = fmt.Sprint(usedVal[i])
	}
	return fmt.Sprintf("квадраты по модулю %d: %s; показатель %d = %s, поэтому %d^%d ≡ %s ≡ %d (mod %d)",
		mod, strings.Join(squares, ", "), exp, strings.Join(pows, " + "), base, exp, strings.Join(vals, "·"), modPow(base, exp, mod), mod)
}

// primitiveRoot возвращает случайный первообразный корень по модулю простого p.
func primitiveRoot(r interface{ IntN(int) int }, p int) int {
	var roots []int
	for g := 2; g <= p-2; g++ {
		seen := map[int]bool{}
		x := 1
		for i := 0; i < p-1; i++ {
			x = x * g % p
			seen[x] = true
		}
		if len(seen) == p-1 {
			roots = append(roots, g)
		}
	}
	return roots[r.IntN(len(roots))]
}

var (
	dhPrimes  = []int{23, 29, 31, 37, 41, 43, 47, 53, 59, 61}
	rsaPrimes = []int{5, 7, 11, 13, 17, 19, 23}
	rsaExps   = []int{3, 5, 7, 11, 13, 17}
)

func (f modexpDH) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC006)
	var stem, explain string
	var answer int
	switch r.IntN(4) {
	case 0:
		p := itembank.Pick(r, dhPrimes)
		g := primitiveRoot(r, p)
		a := itembank.IntBetween(r, 2, p-2)
		answer = modPow(g, a, p)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("В протоколе Диффи — Хеллмана участники договорились о модуле p = %d и основании g = %d. Секретное число Алисы a = %d. Какое открытое значение A = g^a mod p она отправит Бобу?", p, g, a),
			fmt.Sprintf("Обмен ключами Диффи — Хеллмана: p = %d, g = %d. Клиент выбрал закрытое число %d. Вычислите его открытое значение g^%d mod %d.", p, g, a, a, p),
		})
		explain = fmt.Sprintf("Открытое значение — это g^a mod p, вычисляется повторным возведением в квадрат. Вычисление: %s. "+
			"Типичные ошибки — брать остаток только в конце (переполнение и арифметические ошибки) или перепутать основание и показатель.", powTrace(g, a, p))
	case 1:
		p := itembank.Pick(r, dhPrimes)
		g := primitiveRoot(r, p)
		a := itembank.IntBetween(r, 2, p-2)
		b := itembank.IntBetween(r, 2, p-2)
		pubB := modPow(g, b, p)
		answer = modPow(pubB, a, p)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Диффи — Хеллман с p = %d и g = %d. Алиса хранит секрет a = %d и получила от Боба его открытое значение B = %d. Какой общий секрет s = B^a mod p вычислит Алиса?", p, g, a, pubB),
			fmt.Sprintf("В обмене Диффи — Хеллмана (p = %d, g = %d) ваш закрытый ключ равен %d, а открытое значение собеседника — %d. Чему равен общий секрет?", p, g, a, pubB),
		})
		explain = fmt.Sprintf("Общий секрет — открытое значение собеседника в степени своего секрета: s = B^a mod p = g^(ab) mod p. Вычисление: %s. "+
			"Типичные ошибки — перемножить A и B, вычислить g^(a+b) или возвести B в степень g.", powTrace(pubB, a, p))
	case 2:
		pq := itembank.PickN(r, rsaPrimes, 2)
		p, q := pq[0], pq[1]
		n, phi := p*q, (p-1)*(q-1)
		var es []int
		for _, e := range rsaExps {
			if e < phi && gcd(e, phi) == 1 {
				es = append(es, e)
			}
		}
		e := itembank.Pick(r, es)
		m := itembank.IntBetween(r, 2, n-2)
		answer = modPow(m, e, n)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Учебный RSA: p = %d, q = %d (n = %d), открытая экспонента e = %d. Зашифруйте сообщение m = %d: c = m^e mod n. Укажите c.", p, q, n, e, m),
			fmt.Sprintf("Открытый ключ RSA: n = %d (n = %d·%d), e = %d. Чему равен шифртекст для числа %d?", n, p, q, e, m),
		})
		explain = fmt.Sprintf("Шифрование RSA — возведение в степень e по модулю n = p·q (не по модулю φ(n)). Вычисление: %s. "+
			"Типичные ошибки — вычислять по модулю φ(n) = %d или перепутать m и e.", powTrace(m, e, n), phi)
	default:
		pq := itembank.PickN(r, rsaPrimes, 2)
		p, q := pq[0], pq[1]
		n, phi := p*q, (p-1)*(q-1)
		var es []int
		for _, e := range rsaExps {
			if e < phi && gcd(e, phi) == 1 && modInverse(e, phi) != e {
				es = append(es, e)
			}
		}
		e := itembank.Pick(r, es)
		answer = modInverse(e, phi)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Учебный RSA: p = %d, q = %d, открытая экспонента e = %d. Найдите закрытую экспоненту d — наименьшее положительное число, для которого e·d ≡ 1 (mod φ(n)).", p, q, e),
			fmt.Sprintf("Для RSA с простыми %d и %d выбрано e = %d. Вычислите d = e⁻¹ mod φ(n), где φ(n) = (p − 1)(q − 1); ответ — наименьшее положительное d.", p, q, e),
		})
		explain = fmt.Sprintf("φ(n) = (p − 1)(q − 1) = %d·%d = %d. Нужно d с %d·d ≡ 1 (mod %d): %d·%d = %d = %d·%d + 1. Итог d = %d. "+
			"Типичная ошибка — искать обратный элемент по модулю n = %d вместо φ(n).", p-1, q-1, phi, e, phi, e, answer, e*answer, (e*answer-1)/phi, phi, answer, n)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: explain,
	}, nil
}

// tokenTTL — истечение токена: отклонённые запросы, число обновлений, влияние рассинхрона часов.
type tokenTTL struct{}

func (tokenTTL) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.auth_protocols.token-ttl.v1", Spec: itembank.SpecSecurity, Topic: "auth_protocols",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: -0.6, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"jwt", "session", "ttl"},
	}
}

func (f tokenTTL) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC007)
	var stem, explain string
	var answer int
	switch r.IntN(3) {
	case 0:
		issued := time.Date(2026, 1, 1, itembank.IntBetween(r, 8, 18), r.IntN(60), r.IntN(60), 0, time.UTC)
		ttl := itembank.Pick(r, []int{5, 10, 15, 30, 45, 60})
		leeway := itembank.Pick(r, []int{0, 30, 60})
		limit := ttl*60 + leeway
		n := itembank.IntBetween(r, 5, 6)
		var offsets []int
		rejected := 0
		for try := 0; try < 50; try++ {
			seen := map[int]bool{}
			offsets = offsets[:0]
			for len(offsets) < n {
				off := itembank.IntBetween(r, 30, ttl*60*17/10)
				if !seen[off] {
					seen[off] = true
					offsets = append(offsets, off)
				}
			}
			sort.Ints(offsets)
			rejected = 0
			for _, off := range offsets {
				if off >= limit {
					rejected++
				}
			}
			if rejected >= 1 && rejected <= n-1 {
				break
			}
		}
		answer = rejected
		var times, trace []string
		for _, off := range offsets {
			t := issued.Add(time.Duration(off) * time.Second).Format("15:04:05")
			times = append(times, t)
			if off >= limit {
				trace = append(trace, t+" — 401")
			} else {
				trace = append(trace, t+" — принят")
			}
		}
		deadline := issued.Add(time.Duration(limit) * time.Second).Format("15:04:05")
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Сервер выдал access-токен в %s (UTC) со сроком жизни %d минут: exp = iat + %d мин. Токен принимается, пока now < exp + leeway, где leeway = %d с. Запросы с этим токеном пришли в %s. Сколько из них будут отклонены с кодом 401?",
				issued.Format("15:04:05"), ttl, ttl, leeway, strings.Join(times, ", ")),
			fmt.Sprintf("JWT с iat = %s и временем жизни %d мин проверяется с допуском на рассинхрон часов %d с (принимается строго до exp + %d с). Клиент прислал запросы в моменты %s. Укажите число запросов, получивших 401.",
				issued.Format("15:04:05"), ttl, leeway, leeway, strings.Join(times, ", ")),
		})
		explain = fmt.Sprintf("exp = %s, с учётом leeway токен принимается строго до %s. Разбор: %s. Отклонено %d. "+
			"Типичные ошибки — забыть про leeway или считать токен действительным в момент, равный границе (условие строгое).",
			issued.Add(time.Duration(ttl)*time.Minute).Format("15:04:05"), deadline, strings.Join(trace, "; "), rejected)
	case 1:
		ttl := itembank.Pick(r, []int{5, 10, 15, 20, 30})
		step := itembank.Pick(r, []int{1, 2, 3, 5})
		dur := itembank.IntBetween(r, ttl+5, 4*ttl)
		exp := ttl
		var moments []string
		for t := 0; t <= dur; t += step {
			if t >= exp {
				moments = append(moments, fmt.Sprint(t))
				exp = t + ttl
			}
		}
		answer = len(moments)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Клиент получил access-токен при входе (момент 0) со сроком жизни %d минут; токен истёк, если с его выдачи прошло ≥ %d минут. Запросы уходят каждые %d минут начиная с момента 0 и до момента %d включительно. Перед запросом с истёкшим токеном клиент сначала обновляет его по refresh-токену (новый живёт %d минут с момента получения). Сколько раз клиент обновит токен?",
				ttl, ttl, step, dur, ttl),
			fmt.Sprintf("Сессия длится %d минут: запросы в моменты 0, %d, %d, … (шаг %d минут, последний — в момент %d). Access-токен живёт %d минут с момента выдачи и считается истёкшим, когда прошло не меньше %d минут; первый токен выдан в момент 0, при истёкшем токене клиент получает новый перед запросом. Сколько обновлений потребуется?",
				dur, step, 2*step, step, dur, ttl, ttl),
		})
		explain = fmt.Sprintf("Новый токен живёт %d минут от момента обновления, а не от предыдущего истечения. Обновления происходят перед запросами в моменты: %s — всего %d. "+
			"Типичная ошибка — делить длительность сессии на срок жизни (%d/%d) без учёта дискретности запросов.",
			ttl, strings.Join(moments, ", "), answer, dur, ttl)
	default:
		ttl := itembank.Pick(r, []int{5, 10, 15, 30, 60})
		leeway := itembank.Pick(r, []int{0, 30, 60})
		skew := itembank.Pick(r, []int{15, 45, 90, 120})
		ahead := r.IntN(2) == 0
		if ahead {
			answer = ttl*60 + leeway - skew
		} else {
			answer = ttl*60 + leeway + skew
		}
		dir := "отстают от часов клиента"
		sign := "+"
		if ahead {
			dir = "спешат относительно часов клиента"
			sign = "−"
		}
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Клиент получил токен со сроком жизни %d минут (exp = iat + %d мин по часам сервера); сервер принимает токен, пока его now < exp + leeway, leeway = %d с. Часы сервера %s на %d с. Через сколько секунд после выдачи по часам клиента токен перестанет приниматься?",
				ttl, ttl, leeway, dir, skew),
			fmt.Sprintf("Токен живёт %d мин, допуск на рассинхрон leeway = %d с (условие приёма: now_сервера < exp + leeway). Известно, что часы сервера %s на %d с. Сколько секунд с момента выдачи (по часам клиента) токен будет действителен?",
				ttl, leeway, dir, skew),
		})
		explain = fmt.Sprintf("Граница по часам сервера — %d·60 + %d = %d с после выдачи. Часы сервера %s на %d с, поэтому по часам клиента граница сдвигается: %d %s %d = %d с. "+
			"Типичная ошибка — сдвигать границу в другую сторону: когда сервер спешит, токен истекает для клиента раньше.",
			ttl, leeway, ttl*60+leeway, dir, skew, ttl*60+leeway, sign, skew, answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: explain,
	}, nil
}

// authFlowOrder — порядок шагов протокола аутентификации: OAuth 2.0 Authorization Code + PKCE или WebAuthn.
type authFlowOrder struct{}

func (authFlowOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.auth_protocols.auth-flow-order.v1", Spec: itembank.SpecSecurity, Topic: "auth_protocols",
		Grade: itembank.GradeSenior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.5, B: 0.6, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"oauth2", "pkce", "webauthn"},
	}
}

type authFlow struct {
	title string
	steps func(app, idp string) []string
	why   string
}

var authFlows = []authFlow{
	{
		title: "OAuth 2.0 Authorization Code с PKCE",
		steps: func(app, idp string) []string {
			return []string{
				fmt.Sprintf("%s генерирует случайный `code_verifier` и вычисляет `code_challenge` = BASE64URL(SHA-256(code_verifier))", app),
				fmt.Sprintf("%s перенаправляет браузер на `https://%s/authorize` с `response_type=code`, `client_id`, `redirect_uri`, `state`, `code_challenge` и `code_challenge_method=S256`", app, idp),
				fmt.Sprintf("Пользователь вводит учётные данные на %s и подтверждает запрошенные разрешения", idp),
				fmt.Sprintf("%s перенаправляет браузер на `redirect_uri` приложения с параметрами `code` и `state`", idp),
				fmt.Sprintf("%s сверяет полученный `state` с сохранённым перед редиректом значением", app),
				fmt.Sprintf("%s отправляет POST на `https://%s/token` с `grant_type=authorization_code`, `code`, `redirect_uri` и `code_verifier`", app, idp),
				fmt.Sprintf("%s проверяет, что SHA-256(code_verifier) совпадает с сохранённым `code_challenge`, и выдаёт `access_token` и `refresh_token`", idp),
				fmt.Sprintf("%s вызывает API ресурса с заголовком `Authorization: Bearer <access_token>`", app),
				fmt.Sprintf("После истечения `access_token` %s получает новый через POST на `/token` с `grant_type=refresh_token`", app),
			}
		},
		why: "code_verifier создаётся до редиректа, потому что его хеш уходит в /authorize; state проверяется сразу после возврата, до обмена кода; сам code_verifier раскрывается только при обмене на /token, где сервер сверяет его хеш с code_challenge; обновление по refresh_token возможно лишь после выдачи токенов. Типичные ошибки — отправлять code_verifier в /authorize или проверять state после запроса к /token.",
	},
	{
		title: "WebAuthn (аутентификация по ключу)",
		steps: func(app, idp string) []string {
			return []string{
				fmt.Sprintf("Сервер %s генерирует случайный `challenge` и отправляет его браузеру вместе с `rpId` и списком `allowCredentials`", idp),
				fmt.Sprintf("Скрипт %s вызывает `navigator.credentials.get()` с полученным `challenge`", app),
				"Аутентификатор запрашивает подтверждение присутствия пользователя (PIN или биометрия)",
				"Аутентификатор подписывает `authenticatorData` и хеш `clientDataJSON` закрытым ключом и увеличивает `signCount`",
				fmt.Sprintf("Браузер возвращает серверу %s `credentialId`, `authenticatorData`, `clientDataJSON` и `signature`", idp),
				"Сервер проверяет `origin` и `challenge` в `clientDataJSON`, затем подпись открытым ключом, сохранённым при регистрации",
				"Сервер убеждается, что `signCount` больше сохранённого, и создаёт сессию пользователя",
			}
		},
		why: "challenge всегда создаёт сервер до вызова navigator.credentials.get(); подпись появляется только после подтверждения присутствия пользователя; сервер сначала сверяет origin и challenge, затем подпись и лишь потом счётчик и создаёт сессию. Типичные ошибки — ставить проверку подписи до получения ответа браузера или создавать сессию до проверки challenge.",
	},
}

func (f authFlowOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC008)
	flow := authFlows[itembank.Pick(r, []int{0, 0, 1})]
	svc := strings.ReplaceAll(itembank.Ident(r, "service"), "-", "")
	app := "«" + strings.ToUpper(svc[:1]) + svc[1:] + "»"
	kind := itembank.Pick(r, []string{"мобильного", "одностраничного (SPA)", "браузерного"})
	idp := itembank.Pick(r, []string{"auth.", "id.", "login."}) + svc + ".example"
	all := flow.steps(app, idp)
	n := itembank.IntBetween(r, 5, 6)
	idx := make([]int, len(all))
	for i := range idx {
		idx[i] = i
	}
	chosen := itembank.PickN(r, idx, n)
	sort.Ints(chosen)
	steps := make([]string, n)
	for i, j := range chosen {
		steps[i] = all[j]
	}
	options, order := itembank.OrderingOptions(r, steps)
	numbered := make([]string, n)
	for i, s := range steps {
		numbered[i] = fmt.Sprintf("%d) %s", i+1, s)
	}
	stems := []string{
		fmt.Sprintf("Ниже в перемешанном порядке перечислены шаги протокола %s для %sприложения %s и сервера `%s`. Расставьте их в правильной последовательности, начиная с первого.", flow.title, kind+" ", app, idp),
		fmt.Sprintf("Расположите шаги %s в хронологическом порядке (сверху — самый ранний). Участники: %s приложение %s, сервер `%s`.", flow.title, kind, app, idp),
		fmt.Sprintf("Протокол %s. Шаги ниже перепутаны — укажите их верный порядок от начала к концу. Приложение — %s (%s), сервер — `%s`.", flow.title, app, kind, idp),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        itembank.Pick(r, stems),
		Options:     options,
		Answer:      itembank.Answer{Order: order},
		Explanation: fmt.Sprintf("Верный порядок: %s. Почему именно так: %s", strings.Join(numbered, "; "), flow.why),
	}, nil
}

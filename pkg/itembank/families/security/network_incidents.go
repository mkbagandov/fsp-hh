package security

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(cidrBasics{})
	itembank.Register(firewallFirstMatch{})
	itembank.Register(affectedHosts{})
	itembank.Register(timelineOrder{})
}

// b8 приводит значение из [0, 255] к байту (явная маска — для статического анализа).
func b8(x int) byte { return byte(x & 0xFF) }

// plural подбирает форму существительного к числу: 1 бит, 2 бита, 5 бит.
func plural(n int, one, few, many string) string {
	n100, n10 := n%100, n%10
	switch {
	case n100 >= 11 && n100 <= 14:
		return fmt.Sprintf("%d %s", n, many)
	case n10 == 1:
		return fmt.Sprintf("%d %s", n, one)
	case n10 >= 2 && n10 <= 4:
		return fmt.Sprintf("%d %s", n, few)
	default:
		return fmt.Sprintf("%d %s", n, many)
	}
}

func bitsRU(n int) string  { return plural(n, "бит", "бита", "бит") }
func addrsRU(n int) string { return plural(n, "адрес", "адреса", "адресов") }

func ipFromUint(u uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], u)
	return netip.AddrFrom4(b)
}

func uintFromIP(a netip.Addr) uint32 {
	b := a.As4()
	return binary.BigEndian.Uint32(b[:])
}

// randomPrivateIP — случайный адрес из частных диапазонов (RFC 1918).
func randomPrivateIP(r *rand.Rand) netip.Addr {
	switch r.IntN(3) {
	case 0:
		return netip.AddrFrom4([4]byte{10, b8(itembank.IntBetween(r, 1, 99)), b8(r.IntN(256)), b8(itembank.IntBetween(r, 1, 254))})
	case 1:
		return netip.AddrFrom4([4]byte{172, b8(itembank.IntBetween(r, 16, 31)), b8(r.IntN(256)), b8(itembank.IntBetween(r, 1, 254))})
	default:
		return netip.AddrFrom4([4]byte{192, 168, b8(r.IntN(256)), b8(itembank.IntBetween(r, 1, 254))})
	}
}

// cidrBasics — арифметика CIDR: размер блока, принадлежность адресов, число подсетей.
type cidrBasics struct{}

func (cidrBasics) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.network_security.cidr-basics.v1", Spec: itembank.SpecSecurity, Topic: "network_security",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.3, B: -0.7, C: 0}, TimeLimit: 90 * time.Second, Tags: []string{"cidr", "ipv4", "acl"},
	}
}

func (f cidrBasics) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC009)
	var stem, explain string
	var answer int
	switch r.IntN(3) {
	case 0:
		bits := itembank.IntBetween(r, 20, 29)
		block := netip.PrefixFrom(randomPrivateIP(r), bits).Masked()
		total := 1 << (32 - bits)
		if r.IntN(2) == 0 {
			answer = total - 2
			stem = itembank.Pick(r, []string{
				fmt.Sprintf("Сколько адресов можно назначить хостам в сети `%s`? Адрес сети и широковещательный адрес не считаются.", block),
				fmt.Sprintf("В правиле ACL указан блок `%s`. Сколько хостов (без адреса сети и широковещательного) может находиться в этой подсети?", block),
			})
			explain = fmt.Sprintf("Маска /%d оставляет %s под хосты: 2^%d = %s, из них адрес сети и широковещательный хостам не назначаются — остаётся %d. "+
				"Типичные ошибки — забыть вычесть два служебных адреса или возвести в степень длину маски вместо остатка до 32.", bits, bitsRU(32-bits), 32-bits, addrsRU(total), answer)
		} else {
			answer = total
			stem = itembank.Pick(r, []string{
				fmt.Sprintf("Сколько всего IPv4-адресов (включая адрес сети и широковещательный) покрывает блок `%s`?", block),
				fmt.Sprintf("Правило межсетевого экрана разрешает трафик с `%s`. Сколько адресов-источников охватывает это правило, считая адрес сети и широковещательный?", block),
			})
			explain = fmt.Sprintf("Маска /%d фиксирует %s, под адреса остаётся %s: 2^%d = %d. "+
				"Типичная ошибка — вычесть два служебных адреса, хотя для правила фильтрации они тоже входят в диапазон.",
				bits, plural(bits, "старший бит", "старших бита", "старших бит"), bitsRU(32-bits), 32-bits, total)
		}
	case 1:
		bits := itembank.IntBetween(r, 22, 28)
		block := netip.PrefixFrom(randomPrivateIP(r), bits).Masked()
		lo := uintFromIP(block.Addr())
		size := uint32(1) << (32 - bits)
		hi := lo + size - 1
		pool := []uint32{
			lo + 1 + r.Uint32N(size-2),
			lo + 1 + r.Uint32N(size-2),
			lo + 1, hi - 1, lo - 1, hi + 1,
			hi + 1 + r.Uint32N(size),
			lo, hi,
		}
		var addrs []netip.Addr
		seen := map[uint32]bool{}
		for _, u := range itembank.Shuffle(r, pool) {
			if !seen[u] {
				seen[u] = true
				addrs = append(addrs, ipFromUint(u))
			}
			if len(addrs) == 6 {
				break
			}
		}
		var list, notes []string
		for _, a := range addrs {
			list = append(list, "`"+a.String()+"`")
			switch {
			case block.Contains(a):
				answer++
				notes = append(notes, a.String()+" — входит")
			case uintFromIP(a) < lo:
				notes = append(notes, a.String()+" — не входит (меньше адреса сети)")
			default:
				notes = append(notes, a.String()+" — не входит (больше широковещательного)")
			}
		}
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Правило ACL ссылается на блок `%s`. Сколько из перечисленных адресов попадают в этот блок (адрес сети и широковещательный считаются входящими)? Адреса: %s.", block, strings.Join(list, ", ")),
			fmt.Sprintf("Подсеть `%s`. Для каждого из адресов %s определите, принадлежит ли он подсети (включая адрес сети и широковещательный), и укажите количество принадлежащих.", block, strings.Join(list, ", ")),
		})
		explain = fmt.Sprintf("Блок %s покрывает адреса с %s по %s (%s). Разбор: %s. Итого входят %d. "+
			"Типичная ошибка — сравнивать только последний октет и не замечать, что при маске короче /24 меняется и третий октет.",
			block, ipFromUint(lo), ipFromUint(hi), addrsRU(int(size)), strings.Join(notes, "; "), answer)
	default:
		from := itembank.IntBetween(r, 16, 24)
		to := from + itembank.IntBetween(r, 2, 6)
		block := netip.PrefixFrom(randomPrivateIP(r), from).Masked()
		answer = 1 << (to - from)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Блок `%s` нужно разбить на подсети /%d для сегментации сети. Сколько подсетей получится?", block, to),
			fmt.Sprintf("Сколько сетей с маской /%d помещается в блок `%s`?", to, block),
		})
		explain = fmt.Sprintf("Каждый дополнительный бит маски удваивает число подсетей: от /%d до /%d добавляется %s, то есть 2^%d = %d подсетей. "+
			"Типичная ошибка — считать разницу масок как число подсетей или делить размеры блоков в хостах с вычетом служебных адресов.", from, to, bitsRU(to-from), to-from, answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: explain,
	}, nil
}

// firewallFirstMatch — какое правило межсетевого экрана сработает первым и сколько пакетов пройдёт.
type firewallFirstMatch struct{}

func (firewallFirstMatch) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.network_security.firewall-first-match.v1", Spec: itembank.SpecSecurity, Topic: "network_security",
		Grade: itembank.GradeSenior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.6, B: 0.7, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"firewall", "acl", "cidr"},
	}
}

// portSpec — порт назначения правила: любой (lo = 0), один порт или диапазон.
type portSpec struct{ lo, hi int }

func (p portSpec) String() string {
	switch p.lo {
	case 0:
		return "any"
	case p.hi:
		return fmt.Sprint(p.lo)
	default:
		return fmt.Sprintf("%d-%d", p.lo, p.hi)
	}
}

func (p portSpec) contains(port int) bool { return p.lo == 0 || (port >= p.lo && port <= p.hi) }

type fwRule struct {
	action, proto string
	src           netip.Prefix
	port          portSpec
}

type fwPacket struct {
	proto string
	src   netip.Addr
	port  int
}

func (ru fwRule) matches(pk fwPacket) bool {
	return (ru.proto == "any" || ru.proto == pk.proto) && ru.src.Contains(pk.src) && ru.port.contains(pk.port)
}

// misses перечисляет критерии, по которым пакет не подошёл под правило.
func (ru fwRule) misses(pk fwPacket) []string {
	var out []string
	if ru.proto != "any" && ru.proto != pk.proto {
		out = append(out, fmt.Sprintf("протокол %s ≠ %s", pk.proto, ru.proto))
	}
	if !ru.src.Contains(pk.src) {
		out = append(out, fmt.Sprintf("адрес %s ∉ %s", pk.src, ru.src))
	}
	if !ru.port.contains(pk.port) {
		out = append(out, fmt.Sprintf("порт %d ∉ %s", pk.port, ru.port))
	}
	return out
}

func (ru fwRule) render(i int) string {
	return fmt.Sprintf("%-3d%-7s%-6s%-20s%s", i, ru.action, ru.proto, ru.src, ru.port)
}

func (pk fwPacket) String() string {
	return fmt.Sprintf("%s от %s на порт %d", pk.proto, pk.src, pk.port)
}

var (
	fwSinglePorts = []int{22, 25, 53, 80, 443, 3306, 5432, 6379, 8080, 8443}
	fwRanges      = []portSpec{{1024, 65535}, {8000, 8999}, {3000, 3999}, {1, 1023}}
)

// fwGen — генератор правил вокруг заданного пакета: полное совпадение и «почти совпадение».
type fwGen struct {
	r    *rand.Rand
	corp int // X в 10.X.0.0/16 — корпоративная сеть
}

func (g fwGen) packet() fwPacket {
	var src netip.Addr
	switch g.r.IntN(4) {
	case 0:
		src = netip.AddrFrom4([4]byte{192, 168, b8(itembank.IntBetween(g.r, 0, 50)), b8(itembank.IntBetween(g.r, 1, 254))})
	case 1:
		src = netip.AddrFrom4([4]byte{172, b8(itembank.IntBetween(g.r, 16, 31)), b8(g.r.IntN(256)), b8(itembank.IntBetween(g.r, 1, 254))})
	default:
		src = netip.AddrFrom4([4]byte{10, b8(g.corp), b8(itembank.IntBetween(g.r, 1, 200)), b8(itembank.IntBetween(g.r, 1, 254))})
	}
	return fwPacket{proto: itembank.Pick(g.r, []string{"tcp", "tcp", "udp"}), src: src, port: itembank.Pick(g.r, fwSinglePorts)}
}

func (g fwGen) matchingSrc(a netip.Addr) netip.Prefix {
	return netip.PrefixFrom(a, itembank.Pick(g.r, []int{24, 24, 16, 8, 0})).Masked()
}

func (g fwGen) missingSrc(a netip.Addr) netip.Prefix {
	b := a.As4()
	for {
		var p netip.Prefix
		switch g.r.IntN(3) {
		case 0: // соседняя /24 в той же /16
			p = netip.PrefixFrom(netip.AddrFrom4([4]byte{b[0], b[1], b8(itembank.IntBetween(g.r, 1, 200)), 0}), 24)
		case 1:
			p = netip.PrefixFrom(netip.AddrFrom4([4]byte{192, 168, b8(itembank.IntBetween(g.r, 0, 50)), 0}), 24)
		default:
			p = netip.PrefixFrom(netip.AddrFrom4([4]byte{172, b8(itembank.IntBetween(g.r, 16, 31)), 0, 0}), itembank.Pick(g.r, []int{16, 12}))
		}
		if p = p.Masked(); !p.Contains(a) {
			return p
		}
	}
}

func (g fwGen) matchingPort(port int) portSpec {
	switch g.r.IntN(3) {
	case 0:
		return portSpec{port, port}
	case 1:
		for _, rg := range itembank.Shuffle(g.r, fwRanges) {
			if rg.contains(port) {
				return rg
			}
		}
		return portSpec{port, port}
	default:
		return portSpec{}
	}
}

func (g fwGen) missingPort(port int) portSpec {
	for {
		var p portSpec
		if g.r.IntN(2) == 0 {
			x := itembank.Pick(g.r, fwSinglePorts)
			p = portSpec{x, x}
		} else {
			p = itembank.Pick(g.r, fwRanges)
		}
		if !p.contains(port) {
			return p
		}
	}
}

func (g fwGen) fullMatch(pk fwPacket) fwRule {
	proto := pk.proto
	if g.r.IntN(4) == 0 {
		proto = "any"
	}
	return fwRule{action: itembank.Pick(g.r, []string{"allow", "deny"}), proto: proto, src: g.matchingSrc(pk.src), port: g.matchingPort(pk.port)}
}

func (g fwGen) nearMiss(pk fwPacket) fwRule {
	ru := g.fullMatch(pk)
	for _, c := range itembank.PickN(g.r, []int{0, 1, 2}, itembank.IntBetween(g.r, 1, 2)) {
		switch c {
		case 0:
			ru.proto = map[string]string{"tcp": "udp", "udp": "tcp"}[pk.proto]
		case 1:
			ru.src = g.missingSrc(pk.src)
		default:
			ru.port = g.missingPort(pk.port)
		}
	}
	return ru
}

func renderRules(rules []fwRule, def string) string {
	var sb strings.Builder
	sb.WriteString("# правила межсетевого экрана: просмотр сверху вниз, первое совпадение\n")
	fmt.Fprintf(&sb, "%-3s%-7s%-6s%-20s%s\n", "#", "action", "proto", "src", "dport")
	for i, ru := range rules {
		sb.WriteString(ru.render(i + 1))
		sb.WriteByte('\n')
	}
	sb.WriteString("default: " + def)
	return sb.String()
}

func (f firewallFirstMatch) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC00A)
	g := fwGen{r: r, corp: itembank.IntBetween(r, 1, 99)}
	def := itembank.Pick(r, []string{"deny", "deny", "allow"})
	var stem, explain string
	var answer int
	var code string
	if r.IntN(3) != 0 {
		pk := g.packet()
		k := itembank.IntBetween(r, 4, 6)
		target := itembank.IntBetween(r, 0, k)
		if target == 0 && r.IntN(3) != 0 {
			target = itembank.IntBetween(r, 2, k)
		}
		rules := make([]fwRule, k)
		for i := range rules {
			switch {
			case i+1 < target:
				rules[i] = g.nearMiss(pk)
			case i+1 == target:
				rules[i] = g.fullMatch(pk)
			case r.IntN(2) == 0:
				rules[i] = g.fullMatch(pk)
			default:
				rules[i] = g.nearMiss(pk)
			}
		}
		// Ответ вычисляется по итоговому списку, а не по плану генерации.
		var trace []string
		later := ""
		for i, ru := range rules {
			if ru.matches(pk) {
				if answer == 0 {
					answer = i + 1
					trace = append(trace, fmt.Sprintf("правило %d подходит по протоколу, адресу и порту → %s", i+1, ru.action))
				} else if later == "" {
					later = fmt.Sprintf(" Правило %d тоже подходит, но стоит ниже и не рассматривается.", i+1)
				}
				continue
			}
			if answer == 0 {
				trace = append(trace, fmt.Sprintf("правило %d не подходит: %s", i+1, strings.Join(ru.misses(pk), ", ")))
			}
		}
		if answer == 0 {
			trace = append(trace, "ни одно правило не подошло → политика по умолчанию ("+def+")")
		}
		code = renderRules(rules, def)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Межсетевой экран без учёта состояния обрабатывает входящий пакет по списку правил сверху вниз и применяет первое подходящее; если ни одно не подошло — действует политика по умолчанию. Пакет: %s. Укажите номер правила, которое примет решение по этому пакету (0, если сработает политика по умолчанию).", pk),
			fmt.Sprintf("В ACL ниже правила проверяются по порядку до первого совпадения по протоколу, адресу источника и порту назначения. Пришёл пакет: %s. Какое по номеру правило решит его судьбу? Если ни одно — ответ 0.", pk),
		})
		explain = fmt.Sprintf("Разбор сверху вниз: %s.%s Типичные ошибки — выбирать самое специфичное правило вместо первого подходящего, забывать, что any покрывает оба протокола, и неверно проверять принадлежность адреса префиксу.",
			strings.Join(trace, "; "), later)
	} else {
		var packets []fwPacket
		var rules []fwRule
		var trace []string
		for try := 0; try < 20; try++ {
			packets = packets[:0]
			for i := 0; i < 4; i++ {
				packets = append(packets, g.packet())
			}
			rules = rules[:0]
			for i := 0; i < itembank.IntBetween(r, 4, 5); i++ {
				pk := itembank.Pick(r, packets)
				if r.IntN(5) < 3 {
					rules = append(rules, g.fullMatch(pk))
				} else {
					rules = append(rules, g.nearMiss(pk))
				}
			}
			answer = 0
			trace = trace[:0]
			for i, pk := range packets {
				verdict, by := def, "политика по умолчанию"
				for j, ru := range rules {
					if ru.matches(pk) {
						verdict, by = ru.action, fmt.Sprintf("правило %d", j+1)
						break
					}
				}
				if verdict == "allow" {
					answer++
				}
				trace = append(trace, fmt.Sprintf("P%d (%s) — %s → %s", i+1, pk, by, verdict))
			}
			if answer >= 1 && answer <= 3 {
				break
			}
		}
		var sb strings.Builder
		sb.WriteString(renderRules(rules, def))
		sb.WriteString("\n\n# пакеты\n")
		for i, pk := range packets {
			fmt.Fprintf(&sb, "P%d  %-4s %-16s -> :%d\n", i+1, pk.proto, pk.src, pk.port)
		}
		code = strings.TrimRight(sb.String(), "\n")
		stem = itembank.Pick(r, []string{
			"Межсетевой экран применяет к каждому входящему пакету первое подходящее правило (сверху вниз), а при отсутствии совпадений — политику по умолчанию. Сколько из четырёх пакетов ниже будут пропущены (allow)?",
			"Ниже список правил фильтрации (проверка до первого совпадения по протоколу, адресу источника и порту назначения) и четыре пакета. Сколько пакетов пройдёт через экран?",
		})
		explain = fmt.Sprintf("Для каждого пакета ищется первое совпадение: %s. Пропущено %d. "+
			"Типичные ошибки — продолжать просмотр после первого совпадения, игнорировать политику по умолчанию или считать, что deny-правило ниже отменяет allow выше.",
			strings.Join(trace, "; "), answer)
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Code:        &itembank.CodeBlock{Lang: "text", Source: code},
		Answer:      itembank.Answer{Number: float64(answer)},
		Explanation: explain,
	}, nil
}

// affectedHosts — разбор журнала sshd: сколько хостов и учётных записей затронуто подбором пароля.
type affectedHosts struct{}

func (affectedHosts) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.incident_forensics.affected-hosts.v1", Spec: itembank.SpecSecurity, Topic: "incident_forensics",
		Grade: itembank.GradeMiddle, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: 0.1, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"logs", "ssh", "bruteforce"},
	}
}

type sshEvent struct {
	host, user, src string
	ok              bool
}

var (
	sshHosts = []string{"web-01", "web-02", "api-01", "db-01", "bastion", "cache-01", "app-03", "build-02"}
	sshUsers = []string{"deploy", "root", "admin", "backup", "ci", "ops"}
)

func (f affectedHosts) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC00B)
	hosts := itembank.PickN(r, sshHosts, itembank.IntBetween(r, 3, 4))
	users := itembank.PickN(r, sshUsers, 3)
	attacker := itembank.Pick(r, []string{"203.0.113.", "198.51.100."}) + fmt.Sprint(itembank.IntBetween(r, 10, 250))
	internal := fmt.Sprintf("10.0.%d.%d", itembank.IntBetween(r, 1, 9), itembank.IntBetween(r, 2, 250))
	var chains [][]sshEvent
	for _, h := range hosts {
		u := itembank.Pick(r, users)
		var chain []sshEvent
		switch itembank.Pick(r, []int{0, 0, 1, 2, 3}) {
		case 0: // подбор пароля с успехом
			for i := 0; i < itembank.IntBetween(r, 2, 3); i++ {
				chain = append(chain, sshEvent{h, u, attacker, false})
			}
			chain = append(chain, sshEvent{h, u, attacker, true})
		case 1: // только неудачные попытки
			for i := 0; i < itembank.IntBetween(r, 1, 2); i++ {
				chain = append(chain, sshEvent{h, u, attacker, false})
			}
		case 2: // успех с первой-второй попытки
			if r.IntN(2) == 0 {
				chain = append(chain, sshEvent{h, u, attacker, false})
			}
			chain = append(chain, sshEvent{h, u, attacker, true})
		default: // легитимный вход изнутри
			chain = append(chain, sshEvent{h, u, internal, true})
		}
		chains = append(chains, chain)
	}
	for i := 0; i < itembank.IntBetween(r, 1, 2); i++ {
		chains = append(chains, []sshEvent{{itembank.Pick(r, hosts), itembank.Pick(r, users), internal, true}})
	}
	var events []sshEvent
	for len(chains) > 0 {
		i := r.IntN(len(chains))
		events = append(events, chains[i][0])
		chains[i] = chains[i][1:]
		if len(chains[i]) == 0 {
			chains = slices.Delete(chains, i, i+1)
		}
	}
	// Журнал.
	t := time.Date(2026, time.Month(itembank.IntBetween(r, 1, 12)), itembank.IntBetween(r, 1, 28), r.IntN(24), r.IntN(60), r.IntN(60), 0, time.UTC)
	pids := map[string]int{}
	var lines []string
	for _, e := range events {
		t = t.Add(time.Duration(itembank.IntBetween(r, 5, 120)) * time.Second)
		if pids[e.host] == 0 {
			pids[e.host] = itembank.IntBetween(r, 1000, 9999)
		}
		what := "Failed password"
		switch {
		case e.ok && e.src == internal:
			what = "Accepted publickey"
		case e.ok:
			what = "Accepted password"
		}
		lines = append(lines, fmt.Sprintf("%s %s sshd[%d]: %s for %s from %s port %d ssh2",
			t.Format("2006-01-02T15:04:05Z"), e.host, pids[e.host], what, e.user, e.src, itembank.IntBetween(r, 40000, 60000)))
	}
	// Статистика по хостам — по итоговому журналу.
	type stat struct {
		failsBefore, fails int
		ok                 bool
		users              map[string]bool
		internalOK         bool
	}
	stats := map[string]*stat{}
	for _, h := range hosts {
		stats[h] = &stat{users: map[string]bool{}}
	}
	for _, e := range events {
		s := stats[e.host]
		switch {
		case e.src != attacker:
			s.internalOK = true
		case !e.ok:
			s.fails++
			if !s.ok {
				s.failsBefore++
			}
		default:
			s.ok = true
			s.users[e.user] = true
		}
	}
	var compromised, brute, hostsWithAttacker, hostsWithFails []string
	usersSet := map[string]bool{}
	var summary []string
	for _, h := range hosts {
		s := stats[h]
		if s.fails > 0 || s.ok {
			hostsWithAttacker = append(hostsWithAttacker, h)
		}
		if s.fails > 0 {
			hostsWithFails = append(hostsWithFails, h)
		}
		if s.ok {
			compromised = append(compromised, h)
			for u := range s.users {
				usersSet[u] = true
			}
			if s.failsBefore >= 2 {
				brute = append(brute, h)
			}
		}
		var parts []string
		if s.fails > 0 {
			parts = append(parts, plural(s.fails, "неудачная попытка", "неудачные попытки", "неудачных попыток")+" с "+attacker)
		}
		if s.ok {
			var us []string
			for u := range s.users {
				us = append(us, u)
			}
			sort.Strings(us)
			after := "с первой попытки"
			if s.failsBefore > 0 {
				after = "после " + plural(s.failsBefore, "неудачной", "неудачных", "неудачных")
			}
			parts = append(parts, fmt.Sprintf("успешный вход %s с %s %s", strings.Join(us, ", "), attacker, after))
		}
		if s.internalOK {
			parts = append(parts, "успешный вход по ключу с внутреннего адреса "+internal)
		}
		summary = append(summary, h+": "+strings.Join(parts, "; "))
	}
	var stem string
	var answer int
	variant := r.IntN(4)
	if variant == 3 && len(hostsWithAttacker) == 0 {
		variant = 0
	}
	switch variant {
	case 0:
		answer = len(compromised)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Ниже фрагмент журнала sshd, собранный с нескольких хостов. Адрес `%s` внешний и в компании не используется. На скольких различных хостах с этого адреса был выполнен успешный вход?", attacker),
			fmt.Sprintf("При расследовании выгружены записи sshd. Внешний адрес `%s` принадлежит атакующему. Сколько хостов скомпрометировано, то есть имеют хотя бы один успешный вход с этого адреса?", attacker),
		})
	case 1:
		answer = len(brute)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Журнал sshd с нескольких хостов. Признак успешного подбора пароля: с одного адреса на хост сначала приходят не менее двух неудачных попыток, затем — успешный вход. На скольких хостах виден такой признак для адреса `%s`?", attacker),
			fmt.Sprintf("По записям sshd ниже определите число хостов, где успешному входу с адреса `%s` на том же хосте предшествовали как минимум две неудачные попытки с этого адреса.", attacker),
		})
	case 2:
		answer = len(usersSet)
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Фрагмент журнала sshd. Адрес `%s` внешний, принадлежит атакующему. Сколько различных учётных записей получили успешный вход с этого адреса (на любом хосте)?", attacker),
			fmt.Sprintf("Расследование: записи sshd с нескольких серверов. Сколько разных пользователей следует считать скомпрометированными — то есть под сколькими учётными записями был успешный вход с `%s`?", attacker),
		})
	default:
		pool := hostsWithFails
		if len(pool) == 0 {
			pool = hostsWithAttacker
		}
		h := itembank.Pick(r, pool)
		answer = stats[h].fails
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Ниже журнал sshd с нескольких хостов. Сколько неудачных попыток входа с адреса `%s` зафиксировано на хосте `%s`?", attacker, h),
			fmt.Sprintf("По фрагменту журнала sshd подсчитайте число записей «Failed password» с источником `%s` для хоста `%s`.", attacker, h),
		})
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "text", Source: strings.Join(lines, "\n")},
		Answer: itembank.Answer{Number: float64(answer)},
		Explanation: fmt.Sprintf("Сводка по хостам: %s. Ответ — %d. Типичные ошибки — засчитывать входы по ключу с внутреннего адреса %s как атаку, "+
			"считать хост скомпрометированным по одним лишь неудачным попыткам и смешивать события разных хостов с одинаковым пользователем.",
			strings.Join(summary, ". "), answer, internal),
	}, nil
}

// timelineOrder — хронология инцидента по записям из систем с разными форматами времени и зонами.
type timelineOrder struct{}

func (timelineOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "security.incident_forensics.timeline-order.v1", Spec: itembank.SpecSecurity, Topic: "incident_forensics",
		Grade: itembank.GradeSenior, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.5, B: 0.8, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"logs", "timezones"},
	}
}

type pgZone struct {
	abbr   string
	offset int // часы относительно UTC
}

var pgZones = []pgZone{{"EST", -5}, {"CET", 1}, {"MSK", 3}, {"PST", -8}}

func utcOffset(hours int) string {
	sign := "+"
	if hours < 0 {
		sign = "−"
		hours = -hours
	}
	return fmt.Sprintf("UTC%s%02d:00", sign, hours)
}

func (f timelineOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0x5EC00C)
	nginxOff := itembank.Pick(r, []int{3, 2, 5, -3})
	nginxZone := time.FixedZone("local", nginxOff*3600)
	pg := itembank.Pick(r, pgZones)
	pgLoc := time.FixedZone(pg.abbr, pg.offset*3600)
	base := time.Date(2026, time.Month(itembank.IntBetween(r, 1, 12)), itembank.IntBetween(r, 1, 28), itembank.IntBetween(r, 9, 14), r.IntN(60), r.IntN(60), 0, time.UTC)
	// Пять моментов с интервалом не меньше двух минут.
	var mins []int
	for {
		mins = itembank.PickN(r, []int{0, 2, 4, 6, 9, 12, 15, 19, 23, 28, 33, 39, 45, 52, 60, 68, 75}, 5)
		sort.Ints(mins)
		if mins[0] == 0 || r.IntN(2) == 0 {
			break
		}
	}
	times := make([]time.Time, 5)
	for i, m := range mins {
		times[i] = base.Add(time.Duration(m)*time.Minute + time.Duration(r.IntN(60))*time.Second)
	}
	times = itembank.Shuffle(r, times)
	times[4] = times[4].Add(time.Duration(r.IntN(1000)) * time.Millisecond)
	svc := itembank.Ident(r, "service")
	user := itembank.Ident(r, "name_lat")
	host := itembank.Pick(r, sshHosts)
	extIP := fmt.Sprintf("203.0.113.%d", itembank.IntBetween(r, 10, 250))
	intIP := fmt.Sprintf("10.0.%d.%d", itembank.IntBetween(r, 1, 9), itembank.IntBetween(r, 2, 250))
	type entry struct {
		t    time.Time
		src  string
		line string
	}
	entries := []entry{
		{times[0], "nginx", fmt.Sprintf("%s - - [%s] \"%s HTTP/1.1\" %d %d", extIP, times[0].In(nginxZone).Format("02/Jan/2006:15:04:05 -0700"),
			itembank.Pick(r, []string{"POST /login", "GET /admin/users", "POST /api/export", "POST /reset-password"}), itembank.Pick(r, []int{200, 302, 401}), itembank.IntBetween(r, 120, 9800))},
		{times[1], "sshd", fmt.Sprintf("%s bastion sshd[%d]: Accepted publickey for %s from %s port %d ssh2", times[1].UTC().Format("Jan _2 15:04:05"),
			itembank.IntBetween(r, 1000, 9999), user, intIP, itembank.IntBetween(r, 40000, 60000))},
		{times[2], "приложение", fmt.Sprintf("{\"ts\":\"%s\",\"level\":\"warn\",\"service\":\"%s\",\"msg\":\"%s\",\"user\":\"%s\"}", times[2].UTC().Format("2006-01-02T15:04:05Z"), svc,
			itembank.Pick(r, []string{"role changed to admin", "api key created", "export job started", "mfa disabled"}), user)},
		{times[3], "EDR", fmt.Sprintf("%d edr host=%s severity=high rule=%s process=%s", times[3].Unix(), host,
			itembank.Pick(r, []string{"archive-and-upload", "credential-dump", "new-scheduled-task", "lateral-movement"}), itembank.Pick(r, []string{"tar", "rclone", "python3", "ssh"}))},
		{times[4], "PostgreSQL", fmt.Sprintf("%s %s LOG:  %s", times[4].In(pgLoc).Format("2006-01-02 15:04:05.000"), pg.abbr,
			itembank.Pick(r, []string{"connection authorized: user=app database=prod", "statement: COPY customers TO STDOUT", "duration: 1842.213 ms  statement: SELECT * FROM payments", "statement: ALTER ROLE app SUPERUSER"}))},
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].t.Before(entries[j].t) })
	inOrder := make([]string, len(entries))
	explainParts := make([]string, len(entries))
	for i, e := range entries {
		inOrder[i] = "`" + e.line + "`"
		explainParts[i] = fmt.Sprintf("%d) %s — %s UTC", i+1, e.src, e.t.UTC().Format("15:04:05"))
	}
	options, order := itembank.OrderingOptions(r, inOrder)
	zones := fmt.Sprintf("nginx пишет локальное время со смещением (%s); sshd на bastion работает в UTC; приложение пишет ISO 8601 в UTC (суффикс Z); EDR пишет unix-время в секундах; PostgreSQL пишет время в зоне %s (%s)",
		utcOffset(nginxOff), pg.abbr, utcOffset(pg.offset))
	stems := []string{
		fmt.Sprintf("При расследовании инцидента собраны пять записей из разных систем. %s. Расставьте события в хронологическом порядке, начиная с самого раннего.", zones),
		fmt.Sprintf("Ниже по одной записи из пяти источников. Форматы времени различаются: %s. Восстановите хронологию — от самого раннего события к самому позднему.", zones),
		fmt.Sprintf("Постройте таймлайн инцидента из пяти записей. Учтите зоны: %s. Первым должно идти самое раннее событие.", zones),
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:    itembank.Pick(r, stems),
		Options: options,
		Answer:  itembank.Answer{Order: order},
		Explanation: fmt.Sprintf("Все метки приводятся к UTC: %s. Типичные ошибки — сравнивать локальные часы разных зон напрямую, "+
			"прибавлять смещение вместо вычитания (локальное время = UTC + смещение, значит UTC = локальное − смещение) и доверять порядку записей в выгрузке.",
			strings.Join(explainParts, "; ")),
	}, nil
}

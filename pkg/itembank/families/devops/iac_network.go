package devops

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"fsp/pkg/itembank"
)

func init() {
	itembank.Register(terraformApplyOrder{})
	itembank.Register(terraformResourceCount{})
	itembank.Register(cidrSplit{})
	itembank.Register(firewallRules{})
}

// hclBlock собирает блок HCL, выравнивая знаки «=» внутри блока, как это делает terraform fmt.
// Каждый аргумент передаётся строкой вида "key = value".
func hclBlock(header string, attrs []string) string {
	width := 0
	for _, a := range attrs {
		if key, _, ok := strings.Cut(a, " = "); ok && len(key) > width {
			width = len(key)
		}
	}
	var sb strings.Builder
	sb.WriteString(header + " {\n")
	for _, a := range attrs {
		key, val, ok := strings.Cut(a, " = ")
		if !ok {
			sb.WriteString("  " + a + "\n")
			continue
		}
		fmt.Fprintf(&sb, "  %-*s = %s\n", width, key, val)
	}
	sb.WriteString("}")
	return sb.String()
}

// ruPlural подбирает форму существительного к числу: one — 1, few — 2–4, many — остальные.
func ruPlural(n int, one, few, many string) string {
	n100, n10 := n%100, n%10
	switch {
	case n100 >= 11 && n100 <= 19:
		return many
	case n10 == 1:
		return one
	case n10 >= 2 && n10 <= 4:
		return few
	default:
		return many
	}
}

// ipString печатает IPv4-адрес, хранящийся в младших 32 битах int.
func ipString(ip int) string {
	return fmt.Sprintf("%d.%d.%d.%d", ip>>24&255, ip>>16&255, ip>>8&255, ip&255)
}

// parseCIDR разбирает "a.b.c.d/n" в адрес и длину префикса.
func parseCIDR(s string) (ip, bits int) {
	addr, b, _ := strings.Cut(s, "/")
	var o [4]int
	for i, part := range strings.Split(addr, ".") {
		o[i], _ = strconv.Atoi(part)
	}
	bits, _ = strconv.Atoi(b)
	return o[0]<<24 | o[1]<<16 | o[2]<<8 | o[3], bits
}

// inCIDR проверяет, принадлежит ли адрес блоку.
func inCIDR(ip int, cidr string) bool {
	base, bits := parseCIDR(cidr)
	mask := (1<<32 - 1) &^ (1<<(32-bits) - 1)
	return ip&mask == base&mask
}

// terraformApplyOrder — порядок создания (или уничтожения) ресурсов по графу зависимостей Terraform.
type terraformApplyOrder struct{}

func (terraformApplyOrder) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.iac.terraform-apply-order.v1", Spec: itembank.SpecDevOps, Topic: "iac",
		Grade: itembank.GradeMiddle, Format: itembank.FormatOrdering,
		IRT: itembank.IRTParams{A: 1.5, B: 0.2, C: 0}, TimeLimit: 150 * time.Second, Tags: []string{"terraform"},
	}
}

// tfChainLink — звено цепочки зависимостей: тип ресурса, варианты имени, аргумент со ссылкой
// на предыдущее звено и аргумент без ссылки (когда звено открывает фрагмент конфигурации).
type tfChainLink struct {
	typ        string
	names      []string
	ref        func(prev string) string
	standalone func(r *rand.Rand) string
}

func tfRef(prefix, suffix string) func(string) string {
	return func(prev string) string { return prefix + prev + suffix }
}

func tfConst(attr string) func(*rand.Rand) string {
	return func(*rand.Rand) string { return attr }
}

// tfChains — цепочки из пяти ресурсов, где каждый зависит ровно от предыдущего.
var tfChains = [][]tfChainLink{
	{ // сеть → подсеть → инстанс → EIP → DNS-запись
		{"aws_vpc", []string{"core", "main", "platform"}, nil, func(r *rand.Rand) string {
			return fmt.Sprintf(`cidr_block = "10.%d.0.0/16"`, itembank.IntBetween(r, 1, 250))
		}},
		{"aws_subnet", []string{"app", "public_a", "workers"}, tfRef("vpc_id = ", ".id"), tfConst("vpc_id = var.vpc_id")},
		{"aws_instance", []string{"api", "bastion", "gateway"}, tfRef("subnet_id = ", ".id"), tfConst("subnet_id = var.subnet_id")},
		{"aws_eip", []string{"api", "public", "ingress"}, tfRef("instance = ", ".id"), tfConst("instance = var.instance_id")},
		{"aws_route53_record", []string{"api", "www", "app"}, tfRef("records = [", ".public_ip]"), tfConst("records = [var.public_ip]")},
	},
	{ // роль IAM → политика → функция (ждёт политику через depends_on) → алиас → URL
		{"aws_iam_role", []string{"lambda", "exec", "worker"}, nil, func(r *rand.Rand) string {
			return fmt.Sprintf(`assume_role_policy = file("%s")`, itembank.Pick(r, []string{"assume-role.json", "trust.json", "lambda-trust.json"}))
		}},
		{"aws_iam_role_policy", []string{"logs", "s3_access", "queue_access"}, tfRef("role = ", ".id"), tfConst("role = var.role_name")},
		{"aws_lambda_function", []string{"api", "handler", "worker"}, tfRef("depends_on = [", "]"), tfConst("role = var.role_arn")},
		{"aws_lambda_alias", []string{"live", "prod", "stable"}, tfRef("function_name = ", ".function_name"), tfConst("function_name = var.function_name")},
		{"aws_lambda_function_url", []string{"public", "api", "webhook"}, tfRef("qualifier = ", ".name"), tfConst(`qualifier = "live"`)},
	},
	{ // группа подсетей → БД → параметр SSM → task definition → сервис ECS
		{"aws_db_subnet_group", []string{"main", "data", "private"}, nil, tfConst("subnet_ids = var.private_subnet_ids")},
		{"aws_db_instance", []string{"primary", "main", "orders"}, tfRef("db_subnet_group_name = ", ".name"), tfConst("db_subnet_group_name = var.db_subnet_group")},
		{"aws_ssm_parameter", []string{"db_host", "database_url", "db_endpoint"}, tfRef("value = ", ".address"), tfConst("value = var.db_host")},
		{"aws_ecs_task_definition", []string{"api", "worker", "web"}, tfRef(`container_definitions = templatefile("task.json.tpl", { db_host = `, ".value })"), tfConst(`container_definitions = file("task.json")`)},
		{"aws_ecs_service", []string{"api", "worker", "web"}, tfRef("task_definition = ", ".arn"), tfConst("task_definition = var.task_definition_arn")},
	},
	{ // сертификат → запись валидации → подтверждение → listener → правило listener
		{"aws_acm_certificate", []string{"api", "wildcard", "public"}, nil, func(r *rand.Rand) string {
			return fmt.Sprintf(`domain_name = "%s.example.com"`, itembank.Pick(r, []string{"api", "app", "www", "auth"}))
		}},
		{"aws_route53_record", []string{"cert_validation", "acm_validation", "validation"}, tfRef("records = [tolist(", ".domain_validation_options)[0].resource_record_value]"), tfConst("records = [var.validation_value]")},
		{"aws_acm_certificate_validation", []string{"api", "cert", "public"}, tfRef("validation_record_fqdns = [", ".fqdn]"), tfConst("validation_record_fqdns = var.validation_fqdns")},
		{"aws_lb_listener", []string{"https", "public", "api"}, tfRef("certificate_arn = ", ".certificate_arn"), tfConst("certificate_arn = var.certificate_arn")},
		{"aws_lb_listener_rule", []string{"api", "static", "health"}, tfRef("listener_arn = ", ".arn"), tfConst("listener_arn = var.listener_arn")},
	},
}

// withDependsOn добавляет адрес в depends_on блока: расширяет существующий список или создаёт новый.
func withDependsOn(attrs []string, addr string) []string {
	out := append([]string{}, attrs...)
	for i, a := range out {
		if strings.HasPrefix(a, "depends_on = [") {
			out[i] = strings.TrimSuffix(a, "]") + ", " + addr + "]"
			return out
		}
	}
	return append(out, "depends_on = ["+addr+"]")
}

func (f terraformApplyOrder) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F20)
	chain := itembank.Pick(r, tfChains)
	start := r.IntN(2)
	window := chain[start : start+4]
	type tfBlock struct {
		addr   string
		header string
		attrs  []string
	}
	blocks := make([]tfBlock, 0, 5)
	for i, link := range window {
		name := itembank.Pick(r, link.names)
		b := tfBlock{addr: link.typ + "." + name, header: fmt.Sprintf(`resource "%s" "%s"`, link.typ, name)}
		if i == 0 {
			b.attrs = []string{link.standalone(r)}
		} else {
			b.attrs = []string{link.ref(blocks[i-1].addr)}
		}
		blocks = append(blocks, b)
	}
	// Ресурс-пауза: его место в графе задано только depends_on, по типу его не угадать.
	pos := r.IntN(4)
	sleepName := itembank.Pick(r, []string{"wait_for_propagation", "iam_propagation", "settle", "cooldown"})
	sleep := tfBlock{addr: "time_sleep." + sleepName, header: fmt.Sprintf(`resource "time_sleep" "%s"`, sleepName), attrs: []string{
		fmt.Sprintf(`create_duration = "%ds"`, itembank.Pick(r, []int{15, 30, 60, 90})),
		"depends_on = [" + blocks[pos].addr + "]",
	}}
	if pos+1 < len(blocks) {
		blocks[pos+1].attrs = withDependsOn(blocks[pos+1].attrs, sleep.addr)
	}
	creation := make([]string, 0, 5)
	for i, b := range blocks {
		creation = append(creation, b.addr)
		if i == pos {
			creation = append(creation, sleep.addr)
		}
	}
	blocks = append(blocks, sleep)
	rendered := make([]string, len(blocks))
	for i, b := range blocks {
		rendered[i] = hclBlock(b.header, b.attrs)
	}
	code := strings.Join(itembank.Shuffle(r, rendered), "\n\n")

	destroy := r.IntN(3) == 0
	order := append([]string{}, creation...)
	if destroy {
		for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
			order[i], order[j] = order[j], order[i]
		}
	}
	texts := make([]string, len(order))
	for i, a := range order {
		texts[i] = "`" + a + "`"
	}
	options, keys := itembank.OrderingOptions(r, texts)

	note := "Зависимость задаётся ссылкой на атрибут другого ресурса или явным `depends_on`; `time_sleep` — ресурс-пауза провайдера hashicorp/time."
	var stem string
	switch {
	case destroy:
		stem = "Все ресурсы из конфигурации ниже уже созданы, и выполняется `terraform destroy`. Расставьте ресурсы в порядке их уничтожения. " + note
	case r.IntN(2) == 0:
		stem = "Конфигурацию Terraform ниже применяют на пустом state командой `terraform apply`. Расставьте ресурсы в порядке их создания. " + note
	default:
		stem = "Ниже — фрагмент конфигурации Terraform (несущественные аргументы опущены). В каком порядке Terraform создаст ресурсы при первом `terraform apply`? " + note
	}
	next := "и от него ничего не зависит, поэтому он создаётся последним"
	if pos+1 < 4 {
		next = fmt.Sprintf("а `%s` через `depends_on` ждёт его завершения", blocks[pos+1].addr)
	}
	explain := fmt.Sprintf("Граф зависимостей (стрелка — «должен существовать раньше»): %s. Порядок объявления блоков в файле значения не имеет: "+
		"Terraform строит граф из ссылок между ресурсами (например, `%s`) и явных `depends_on`. `%s` зависит от `%s` только через `depends_on`, %s. ",
		strings.Join(creation, " → "), blocks[1].attrs[0], sleep.addr, blocks[pos].addr, next)
	if destroy {
		explain += "При `terraform destroy` граф обходится в обратном порядке: сначала уничтожаются зависимые ресурсы, последним — тот, от которого зависели остальные. "
	}
	explain += "Типичная ошибка — расставлять ресурсы по порядку в файле или по «привычной» последовательности для этих типов, не замечая зависимости через ресурс-паузу."
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem,
		Code:        &itembank.CodeBlock{Lang: "hcl", Source: code},
		Options:     options,
		Answer:      itembank.Answer{Order: keys},
		Explanation: explain,
	}, nil
}

// terraformResourceCount — сколько экземпляров ресурсов создаст конфигурация с count и for_each.
type terraformResourceCount struct{}

func (terraformResourceCount) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.iac.terraform-resource-count.v1", Spec: itembank.SpecDevOps, Topic: "iac",
		Grade: itembank.GradeJunior, Format: itembank.FormatNumeric,
		IRT: itembank.IRTParams{A: 1.5, B: -0.6, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"terraform"},
	}
}

// tfCloud — имена типов ресурсов одного провайдера.
type tfCloud struct {
	zones                                               []string
	subnet, instance, bucket, account, record, logGroup string
}

var tfClouds = []tfCloud{
	{[]string{"eu-central-1a", "eu-central-1b", "eu-central-1c"}, "aws_subnet", "aws_instance", "aws_s3_bucket", "aws_iam_user", "aws_route53_record", "aws_cloudwatch_log_group"},
	{[]string{"ru-central1-a", "ru-central1-b", "ru-central1-d"}, "yandex_vpc_subnet", "yandex_compute_instance", "yandex_storage_bucket", "yandex_iam_service_account", "yandex_dns_recordset", "yandex_logging_group"},
}

// tfLocals — значения блока locals, от которых зависят count и for_each.
type tfLocals struct {
	zones   []string
	perZone int
	env     string
	bastion bool
	buckets []string
	users   []string
	admins  []bool
	service string
}

// tfCountTpl — шаблон блока resource: какие locals ему нужны и как он считается.
type tfCountTpl struct {
	locals []string
	render func(c tfCloud, l tfLocals) (block string, n int, why string)
}

func uniqueStrings(xs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

var tfCountTpls = []tfCountTpl{
	{[]string{"zones"}, func(c tfCloud, l tfLocals) (string, int, string) {
		n := len(l.zones)
		return hclBlock(fmt.Sprintf(`resource "%s" "private"`, c.subnet), []string{"count = length(local.zones)"}), n,
			fmt.Sprintf("`%s.private`: `count = length(local.zones)` = %d", c.subnet, n)
	}},
	{[]string{"zones", "replicas_per_zone"}, func(c tfCloud, l tfLocals) (string, int, string) {
		n := l.perZone * len(l.zones)
		return hclBlock(fmt.Sprintf(`resource "%s" "node"`, c.instance), []string{"count = local.replicas_per_zone * length(local.zones)"}), n,
			fmt.Sprintf("`%s.node`: %d × %d = %d", c.instance, l.perZone, len(l.zones), n)
	}},
	{[]string{"create_bastion"}, func(c tfCloud, l tfLocals) (string, int, string) {
		n := 0
		if l.bastion {
			n = 1
		}
		return hclBlock(fmt.Sprintf(`resource "%s" "bastion"`, c.instance), []string{"count = local.create_bastion ? 1 : 0"}), n,
			fmt.Sprintf("`%s.bastion`: `create_bastion = %t`, поэтому count = %d", c.instance, l.bastion, n)
	}},
	{[]string{"buckets"}, func(c tfCloud, l tfLocals) (string, int, string) {
		n := len(uniqueStrings(l.buckets))
		why := fmt.Sprintf("`%s.data`: `for_each = toset(...)` по %d уникальным именам = %d", c.bucket, n, n)
		if n < len(l.buckets) {
			why = fmt.Sprintf("`%s.data`: `toset` из %d имён оставляет %d уникальных", c.bucket, len(l.buckets), n)
		}
		return hclBlock(fmt.Sprintf(`resource "%s" "data"`, c.bucket), []string{"for_each = toset(local.buckets)"}), n, why
	}},
	{[]string{"users"}, func(c tfCloud, l tfLocals) (string, int, string) {
		n := 0
		for _, a := range l.admins {
			if a {
				n++
			}
		}
		return hclBlock(fmt.Sprintf(`resource "%s" "admin"`, c.account), []string{"for_each = { for name, u in local.users : name => u if u.admin }"}), n,
			fmt.Sprintf("`%s.admin`: фильтр `if u.admin` оставляет %d из %d пользователей", c.account, n, len(l.users))
	}},
	{[]string{"env", "zones"}, func(c tfCloud, l tfLocals) (string, int, string) {
		n := 1
		if l.env == "prod" {
			n = len(l.zones)
		}
		return hclBlock(fmt.Sprintf(`resource "%s" "app"`, c.record), []string{`count = local.env == "prod" ? length(local.zones) : 1`}), n,
			fmt.Sprintf("`%s.app`: env = \"%s\", поэтому count = %d", c.record, l.env, n)
	}},
	{nil, func(c tfCloud, l tfLocals) (string, int, string) {
		return hclBlock(fmt.Sprintf(`resource "%s" "app"`, c.logGroup), []string{fmt.Sprintf(`name = "%s-logs"`, l.service)}), 1,
			fmt.Sprintf("`%s.app`: без count и for_each — ровно 1 экземпляр", c.logGroup)
	}},
}

func (f terraformResourceCount) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F21)
	cloud := itembank.Pick(r, tfClouds)
	zones := itembank.PickN(r, cloud.zones, itembank.IntBetween(r, 2, 3))
	sort.Strings(zones)
	buckets := itembank.PickN(r, []string{"logs", "backups", "assets", "uploads", "exports", "archive"}, 3)
	if r.IntN(2) == 0 {
		buckets = append(buckets, itembank.Pick(r, buckets)) // дубликат: toset его уберёт
		buckets = itembank.Shuffle(r, buckets)
	}
	users := itembank.Idents(r, "name_lat", itembank.IntBetween(r, 3, 4))
	admins := make([]bool, len(users))
	for i := range admins {
		admins[i] = r.IntN(2) == 0
	}
	l := tfLocals{
		zones: zones, perZone: itembank.IntBetween(r, 1, 3), env: itembank.Pick(r, []string{"prod", "staging", "dev"}),
		bastion: r.IntN(2) == 0, buckets: buckets, users: users, admins: admins, service: itembank.Ident(r, "service"),
	}
	tpls := itembank.PickN(r, tfCountTpls, itembank.IntBetween(r, 3, 4))
	need := map[string]bool{}
	var blocks, whys []string
	total := 0
	for _, t := range tpls {
		for _, k := range t.locals {
			need[k] = true
		}
		block, n, why := t.render(cloud, l)
		blocks = append(blocks, block)
		whys = append(whys, why)
		total += n
	}
	var attrs []string
	if need["zones"] {
		attrs = append(attrs, "zones = "+jsonArray(zones))
	}
	if need["replicas_per_zone"] {
		attrs = append(attrs, fmt.Sprintf("replicas_per_zone = %d", l.perZone))
	}
	if need["env"] {
		attrs = append(attrs, fmt.Sprintf(`env = "%s"`, l.env))
	}
	if need["create_bastion"] {
		attrs = append(attrs, fmt.Sprintf("create_bastion = %t", l.bastion))
	}
	if need["buckets"] {
		attrs = append(attrs, "buckets = "+jsonArray(buckets))
	}
	if need["users"] {
		parts := make([]string, len(users))
		for i, u := range users {
			parts[i] = fmt.Sprintf("%s = { admin = %t }", u, admins[i])
		}
		attrs = append(attrs, "users = { "+strings.Join(parts, ", ")+" }")
	}
	code := hclBlock("locals", attrs) + "\n\n" + strings.Join(blocks, "\n\n")
	stem := itembank.Pick(r, []string{
		"Конфигурацию Terraform ниже применяют на пустом state. Сколько экземпляров ресурсов будет создано — какое число N покажет `terraform plan` в строке `Plan: N to add, 0 to change, 0 to destroy`?",
		"Ниже — конфигурация Terraform, state пуст. Сколько ресурсов создаст `terraform apply`? Считайте экземпляры ресурсов, а не блоки `resource`.",
		"Какое число окажется в позиции «to add» в итоговой строке `terraform plan` для конфигурации ниже при пустом state?",
	})
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   stem,
		Code:   &itembank.CodeBlock{Lang: "hcl", Source: code},
		Answer: itembank.Answer{Number: float64(total)},
		Explanation: fmt.Sprintf("Считаются экземпляры, а не блоки: %s. Итого %d. `count = 0` означает, что ресурса не будет вовсе; `toset()` убирает дубликаты; "+
			"for-выражение с `if` оставляет только прошедшие фильтр элементы; блок без `count`/`for_each` даёт ровно один экземпляр. "+
			"Типичная ошибка — посчитать число блоков `resource` или учесть повторы в списке для `toset`.", strings.Join(whys, "; "), total),
	}, nil
}

// cidrSplit — нарезка сети на подсети: адрес k-й подсети, её broadcast, последний хост, поиск подсети по адресу.
type cidrSplit struct{}

func (cidrSplit) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.networking.cidr-split.v1", Spec: itembank.SpecDevOps, Topic: "networking",
		Grade: itembank.GradeMiddle, Format: itembank.FormatShortText,
		IRT: itembank.IRTParams{A: 1.5, B: -0.2, C: 0}, TimeLimit: 120 * time.Second, Tags: []string{"networking", "cidr"},
	}
}

func (f cidrSplit) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F22)
	prefix := itembank.Pick(r, []int{22, 23, 24, 24})
	var first, second int
	switch r.IntN(3) {
	case 0:
		first, second = 10, r.IntN(256)
	case 1:
		first, second = 172, itembank.IntBetween(r, 16, 31)
	default:
		first, second = 192, 168
	}
	// Третий октет выровнен по границе префикса: для /22 кратен 4, для /23 — чётный.
	third := r.IntN(1<<(prefix-16)) << (24 - prefix)
	base := first<<24 | second<<16 | third<<8
	newPrefix := prefix + itembank.IntBetween(r, 1, 4)
	size := 1 << (32 - newPrefix)
	nSub := 1 << (newPrefix - prefix)
	network := fmt.Sprintf("%s/%d", ipString(base), prefix)
	intro := itembank.Pick(r, []string{
		fmt.Sprintf("Сеть `%s` делят на равные подсети с маской /%d.", network, newPrefix),
		fmt.Sprintf("Блок `%s` нарезают на подсети /%d одинакового размера.", network, newPrefix),
	})
	k := itembank.IntBetween(r, 2, nSub)
	subnet := base + (k-1)*size
	bcast := subnet + size - 1
	common := fmt.Sprintf("Маска /%d оставляет %d бит на хосты — подсети по %d %s, всего %d %s. Адрес k-й подсети (нумерация с 1) = адрес сети + (k − 1) × %d.",
		newPrefix, 32-newPrefix, size, ruPlural(size, "адрес", "адреса", "адресов"), nSub, ruPlural(nSub, "подсеть", "подсети", "подсетей"), size)
	var question, answer, detail string
	switch r.IntN(4) {
	case 0:
		question = fmt.Sprintf("Какой адрес (в формате `a.b.c.d/nn`) получит %d-я по порядку подсеть, если нумеровать их с 1 по возрастанию адресов?", k)
		answer = fmt.Sprintf("%s/%d", ipString(subnet), newPrefix)
		detail = fmt.Sprintf("Для k = %d: %s + %d × %d = %s + %d адресов = `%s`.", k, ipString(base), k-1, size, ipString(base), (k-1)*size, answer)
	case 1:
		question = fmt.Sprintf("Какой последний адрес, пригодный для хоста, в %d-й подсети (нумерация с 1)? Ответ — IPv4-адрес.", k)
		answer = ipString(bcast - 1)
		detail = fmt.Sprintf("%d-я подсеть — %s/%d, её диапазон %s – %s; последний адрес (%s) широковещательный, последний для хоста — на единицу меньше: `%s`.",
			k, ipString(subnet), newPrefix, ipString(subnet), ipString(bcast), ipString(bcast), answer)
	case 2:
		question = fmt.Sprintf("Какой широковещательный адрес у %d-й подсети (нумерация с 1)? Ответ — IPv4-адрес.", k)
		answer = ipString(bcast)
		detail = fmt.Sprintf("%d-я подсеть начинается с %s; широковещательный адрес — последний в ней: %s + %d − 1 = `%s`.", k, ipString(subnet), ipString(subnet), size, answer)
	default:
		j := itembank.IntBetween(r, 1, nSub)
		subnet = base + (j-1)*size
		bcast = subnet + size - 1
		ip := subnet + itembank.IntBetween(r, 1, size-2)
		question = fmt.Sprintf("В какую из подсетей (ответ в формате `a.b.c.d/nn`) попадает адрес `%s`?", ipString(ip))
		answer = fmt.Sprintf("%s/%d", ipString(subnet), newPrefix)
		detail = fmt.Sprintf("Смещение адреса от начала сети — %d; %d / %d = %d (с нуля), то есть %d-я подсеть: `%s` (диапазон %s – %s).",
			ip-base, ip-base, size, (ip-base)/size, j, answer, ipString(subnet), ipString(bcast))
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:   intro + " " + question,
		Answer: itembank.Answer{Text: answer},
		Explanation: common + " " + detail + " Типичные ошибки — нумеровать подсети с нуля, прибавлять смещение к последнему октету без переноса в третий " +
			"(когда смещение не меньше 256) и путать широковещательный адрес с последним адресом для хоста.",
	}, nil
}

// firewallRules — какие входящие соединения пропустит группа безопасности или сетевой ACL.
type firewallRules struct{}

func (firewallRules) Meta() itembank.Meta {
	return itembank.Meta{
		ID: "devops.networking.firewall-rules.v1", Spec: itembank.SpecDevOps, Topic: "networking",
		Grade: itembank.GradeSenior, Format: itembank.FormatMultiChoice,
		IRT: itembank.IRTParams{A: 1.5, B: 0.7, C: 0}, TimeLimit: 180 * time.Second, Tags: []string{"networking", "security-group", "acl"},
	}
}

type fwRule struct {
	num    int // номер правила в ACL; 0 для группы безопасности
	allow  bool
	proto  string // tcp | udp | all
	lo, hi int
	cidr   string
}

type fwConn struct {
	proto string
	src   int
	port  int
}

func (c fwConn) String() string {
	return fmt.Sprintf("%s с %s на порт %d", strings.ToUpper(c.proto), ipString(c.src), c.port)
}

func (rule fwRule) matches(c fwConn) bool {
	return (rule.proto == "all" || rule.proto == c.proto) && c.port >= rule.lo && c.port <= rule.hi && inCIDR(c.src, rule.cidr)
}

func (rule fwRule) protoText() string {
	if rule.proto == "all" {
		return "любой"
	}
	return strings.ToUpper(rule.proto)
}

func (rule fwRule) portsText() string {
	switch {
	case rule.proto == "all":
		return "все"
	case rule.lo == rule.hi:
		return strconv.Itoa(rule.lo)
	default:
		return fmt.Sprintf("%d–%d", rule.lo, rule.hi)
	}
}

func (rule fwRule) describe() string {
	if rule.proto == "all" {
		return "любой трафик из " + rule.cidr
	}
	return fmt.Sprintf("%s %s из %s", rule.protoText(), rule.portsText(), rule.cidr)
}

func (rule fwRule) key() string {
	return fmt.Sprintf("%t|%s|%d|%d|%s", rule.allow, rule.proto, rule.lo, rule.hi, rule.cidr)
}

type fwPortRange struct{ lo, hi int }

var (
	fwPortPool = []fwPortRange{{22, 22}, {80, 80}, {443, 443}, {3306, 3306}, {5432, 5432}, {6379, 6379}, {8080, 8080}, {9090, 9090}, {1024, 65535}, {8000, 8999}, {30000, 32767}, {10250, 10259}}
	fwUDPPorts = []int{53, 123, 514, 161}
	fwCIDRPool = []string{"10.0.0.0/8", "10.0.0.0/16", "10.0.3.0/24", "10.0.3.128/25", "172.16.0.0/12", "172.16.5.0/24", "192.168.1.0/24", "192.168.0.0/16", "203.0.113.0/24", "203.0.113.64/26", "198.51.100.0/24", "0.0.0.0/0"}
	// Пары «узкий блок внутри широкого» для правил deny/allow на один и тот же порт.
	fwNestedCIDRs = [][2]string{{"10.0.3.0/24", "10.0.0.0/8"}, {"10.0.3.128/25", "10.0.3.0/24"}, {"192.168.1.0/24", "192.168.0.0/16"}, {"203.0.113.64/26", "203.0.113.0/24"}, {"172.16.5.0/24", "172.16.0.0/12"}, {"198.51.100.0/24", "0.0.0.0/0"}, {"10.0.0.0/16", "0.0.0.0/0"}}
)

func randomAllowRule(r *rand.Rand) fwRule {
	if r.IntN(5) == 0 {
		p := itembank.Pick(r, fwUDPPorts)
		return fwRule{allow: true, proto: "udp", lo: p, hi: p, cidr: itembank.Pick(r, fwCIDRPool)}
	}
	p := itembank.Pick(r, fwPortPool)
	return fwRule{allow: true, proto: "tcp", lo: p.lo, hi: p.hi, cidr: itembank.Pick(r, fwCIDRPool)}
}

// randomIPIn возвращает адрес внутри блока или (inside = false) в соседнем блоке того же размера.
func randomIPIn(r *rand.Rand, cidr string, inside bool) int {
	base, bits := parseCIDR(cidr)
	if bits == 0 {
		return itembank.Pick(r, []int{203<<24 | 113<<8, 198<<24 | 51<<16 | 100<<8, 192<<24 | 2<<8}) + itembank.IntBetween(r, 1, 254)
	}
	size := 1 << (32 - bits)
	if !inside {
		base ^= size
	}
	return base + itembank.IntBetween(r, 1, size-2)
}

// fwEvaluate возвращает решение и индекс первого совпавшего правила (−1 — ни одно не совпало).
func fwEvaluate(rules []fwRule, c fwConn) (bool, int) {
	for i, rule := range rules {
		if rule.matches(c) {
			return rule.allow, i
		}
	}
	return false, -1
}

// fwNearMiss объясняет, почему соединение не подошло ни под одно разрешающее правило:
// берёт правило с наибольшим числом совпавших критериев и называет несовпавшие.
func fwNearMiss(rules []fwRule, c fwConn) string {
	best, bestScore := -1, 0
	for i, rule := range rules {
		if !rule.allow || rule.proto == "all" {
			continue
		}
		score := 0
		if rule.proto == c.proto {
			score++
		}
		if c.port >= rule.lo && c.port <= rule.hi {
			score++
		}
		if inCIDR(c.src, rule.cidr) {
			score++
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return "ни одно правило не совпадает ни по протоколу, ни по порту, ни по источнику"
	}
	rule := rules[best]
	var gaps []string
	if rule.proto != c.proto {
		gaps = append(gaps, fmt.Sprintf("протокол %s не совпадает", strings.ToUpper(c.proto)))
	}
	if c.port < rule.lo || c.port > rule.hi {
		gaps = append(gaps, fmt.Sprintf("порт %d вне диапазона", c.port))
	}
	if !inCIDR(c.src, rule.cidr) {
		gaps = append(gaps, fmt.Sprintf("адрес %s не входит в %s", ipString(c.src), rule.cidr))
	}
	return fmt.Sprintf("ближайшее правило «%s»: %s", rule.describe(), strings.Join(gaps, ", "))
}

func fwRulesDistinct(rules []fwRule) bool {
	seen := map[string]bool{}
	for _, rule := range rules {
		if seen[rule.key()] {
			return false
		}
		seen[rule.key()] = true
	}
	return true
}

func (f firewallRules) Generate(seed uint64) (itembank.Instance, error) {
	r := itembank.NewRand(seed, 0xDE0F23)
	svc := itembank.Ident(r, "service")
	nacl := r.IntN(2) == 0
	var rules []fwRule
	var conns []fwConn
	for {
		rules = rules[:0]
		if nacl {
			pair := itembank.Pick(r, fwNestedCIDRs)
			p := itembank.Pick(r, fwPortPool)
			deny := fwRule{allow: false, proto: "tcp", lo: p.lo, hi: p.hi, cidr: pair[0]}
			if r.IntN(3) == 0 {
				deny = fwRule{allow: false, proto: "all", lo: 0, hi: 65535, cidr: pair[0]}
			}
			allow := fwRule{allow: true, proto: "tcp", lo: p.lo, hi: p.hi, cidr: pair[1]}
			if r.IntN(5) < 3 {
				rules = append(rules, deny, allow)
			} else {
				rules = append(rules, allow, deny)
			}
			for len(rules) < 5 {
				rules = append(rules, randomAllowRule(r))
			}
			for i := range rules {
				rules[i].num = 100 + 10*i
			}
		} else {
			for len(rules) < 4 {
				rules = append(rules, randomAllowRule(r))
			}
		}
		if !fwRulesDistinct(rules) {
			continue
		}
		conns = conns[:0]
		seen := map[string]bool{}
		for len(conns) < 6 {
			rule := itembank.Pick(r, rules)
			c := fwConn{proto: rule.proto, port: rule.lo}
			if rule.proto == "all" {
				c.proto = itembank.Pick(r, []string{"tcp", "udp"})
				c.port = itembank.Pick(r, []int{22, 80, 443, 8080, 5432})
			} else if rule.lo != rule.hi {
				c.port = itembank.IntBetween(r, rule.lo, rule.hi)
			}
			switch r.IntN(10) {
			case 0, 1: // порт сразу за границей диапазона
				switch {
				case r.IntN(2) == 0 && rule.hi < 65535:
					c.port = rule.hi + 1
				case rule.lo > 1:
					c.port = rule.lo - 1
				}
			case 2: // другой протокол
				if c.proto == "tcp" {
					c.proto = "udp"
				} else {
					c.proto = "tcp"
				}
			}
			_, bits := parseCIDR(rule.cidr)
			c.src = randomIPIn(r, rule.cidr, r.IntN(10) < 6 || bits == 0)
			if seen[c.String()] {
				continue
			}
			seen[c.String()] = true
			conns = append(conns, c)
		}
		nAllowed, hitDeny := 0, false
		for _, c := range conns {
			ok, idx := fwEvaluate(rules, c)
			if ok {
				nAllowed++
			}
			if idx >= 0 && !rules[idx].allow {
				hitDeny = true
			}
		}
		if nAllowed >= 2 && nAllowed <= 3 && (!nacl || hitDeny) {
			break
		}
	}

	var table strings.Builder
	if nacl {
		table.WriteString("| № | Действие | Протокол | Порты | Источник |\n|---|---|---|---|---|\n")
		for _, rule := range rules {
			action := "allow"
			if !rule.allow {
				action = "deny"
			}
			fmt.Fprintf(&table, "| %d | %s | %s | %s | %s |\n", rule.num, action, rule.protoText(), rule.portsText(), rule.cidr)
		}
		table.WriteString("| * | deny | любой | все | 0.0.0.0/0 |\n")
	} else {
		table.WriteString("| Протокол | Порты | Источник |\n|---|---|---|\n")
		for _, rule := range rules {
			fmt.Fprintf(&table, "| %s | %s | %s |\n", rule.protoText(), rule.portsText(), rule.cidr)
		}
	}
	var correct, wrong, details []string
	for _, c := range conns {
		ok, idx := fwEvaluate(rules, c)
		opt := c.String()
		if ok {
			correct = append(correct, opt)
		} else {
			wrong = append(wrong, opt)
		}
		var line string
		switch {
		case idx < 0 && nacl:
			line = fmt.Sprintf("%s — не совпало ни одно правило (%s), действует запрет по умолчанию", opt, fwNearMiss(rules, c))
		case idx < 0:
			line = fmt.Sprintf("%s — отклонено: %s", opt, fwNearMiss(rules, c))
		case nacl && !ok:
			line = fmt.Sprintf("%s — первым совпадает правило %d (deny): запрещено", opt, rules[idx].num)
			for _, later := range rules[idx+1:] {
				if later.allow && later.matches(c) {
					line += fmt.Sprintf(", хотя правило %d ниже его разрешило бы", later.num)
					break
				}
			}
		case nacl:
			line = fmt.Sprintf("%s — первым совпадает правило %d (allow): разрешено", opt, rules[idx].num)
			for _, later := range rules[idx+1:] {
				if !later.allow && later.matches(c) {
					line += fmt.Sprintf("; правило %d (deny) ниже уже не проверяется", later.num)
					break
				}
			}
		default:
			line = fmt.Sprintf("%s — подходит под правило «%s»: разрешено", opt, rules[idx].describe())
		}
		details = append(details, line)
	}
	options, keys := itembank.ShuffledMulti(r, correct, wrong...)
	var stem, mistakes string
	if nacl {
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("На подсети с сервисом `%s` действует сетевой ACL; правила для входящего трафика — в таблице. Правила проверяются по возрастанию номера, применяется первое совпавшее; строка `*` — запрет по умолчанию. Какие из перечисленных входящих соединений будут пропущены? Выберите все подходящие.", svc),
			fmt.Sprintf("Входящий трафик к подсети сервиса `%s` фильтрует сетевой ACL (таблица ниже): правила просматриваются по возрастанию номера до первого совпадения по протоколу, порту назначения и адресу источника; `*` — запрет по умолчанию. Какие соединения дойдут до сервиса? Выберите все подходящие.", svc),
		})
		mistakes = "Типичные ошибки — проверять правила не по порядку номеров (ACL не ищет «самое подходящее» правило), считать адрес из соседнего блока входящим в CIDR и не замечать границ диапазона портов."
	} else {
		stem = itembank.Pick(r, []string{
			fmt.Sprintf("Для инстанса `%s` настроена группа безопасности; правила для входящего трафика — в таблице. Соединение принимается, если подходит хотя бы под одно правило (протокол, порт назначения и адрес источника), иначе отклоняется. Какие из перечисленных входящих соединений будут приняты? Выберите все подходящие.", svc),
			fmt.Sprintf("Входящий трафик к инстансу `%s` фильтрует группа безопасности (таблица ниже): правила только разрешающие, достаточно совпадения с любым из них по протоколу, порту назначения и источнику; всё остальное отбрасывается. Какие соединения дойдут до инстанса? Выберите все подходящие.", svc),
		})
		mistakes = "Типичные ошибки — считать адрес из соседнего блока (например, 10.0.4.x для 10.0.3.0/24) входящим в CIDR, не замечать границ диапазона портов и игнорировать протокол."
	}
	return itembank.Instance{
		FamilyID: f.Meta().ID, Seed: seed,
		Stem:        stem + "\n\n" + table.String(),
		Options:     options,
		Answer:      itembank.Answer{Keys: keys},
		Explanation: strings.Join(details, "; ") + ". " + mistakes,
	}, nil
}

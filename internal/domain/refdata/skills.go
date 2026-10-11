package refdata

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	"go.yaml.in/yaml/v3"
)

// skillsYAML — справочник навыков с синонимами. Правится руками, проверяется тестом.
//
//go:embed skills.yaml
var skillsYAML []byte

// Skill — элемент справочника навыков.
type Skill struct {
	Code     string   `yaml:"code"`
	Name     string   `yaml:"name"`
	Group    string   `yaml:"group"`
	Specs    []string `yaml:"specs"`
	Topic    string   `yaml:"topic,omitempty"`
	Synonyms []string `yaml:"synonyms"`
}

type skillsFile struct {
	Version int               `yaml:"version"`
	Groups  map[string]string `yaml:"groups"`
	Skills  []Skill           `yaml:"skills"`
}

// SkillIndex — загруженный справочник с индексами по коду и по нормализованным названиям.
type SkillIndex struct {
	Groups  map[string]string
	Skills  []Skill
	byCode  map[string]*Skill
	byAlias map[string]*Skill // нормализованное имя или синоним → навык
}

var (
	skillsOnce sync.Once
	skillsIdx  *SkillIndex
	skillsErr  error
)

// Skills возвращает справочник навыков (загружается один раз).
func Skills() (*SkillIndex, error) {
	skillsOnce.Do(func() {
		skillsIdx, skillsErr = parseSkills(skillsYAML)
	})
	return skillsIdx, skillsErr
}

// MustSkills — как Skills, но паникует при ошибке (для init и тестов).
func MustSkills() *SkillIndex {
	idx, err := Skills()
	if err != nil {
		panic(err)
	}
	return idx
}

func parseSkills(raw []byte) (*SkillIndex, error) {
	var f skillsFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("skills.yaml: %w", err)
	}
	idx := &SkillIndex{
		Groups:  f.Groups,
		Skills:  f.Skills,
		byCode:  make(map[string]*Skill, len(f.Skills)),
		byAlias: make(map[string]*Skill, len(f.Skills)*3),
	}
	for i := range idx.Skills {
		s := &idx.Skills[i]
		if s.Code == "" || s.Name == "" {
			return nil, fmt.Errorf("skills.yaml: skill #%d without code or name", i)
		}
		if _, dup := idx.byCode[s.Code]; dup {
			return nil, fmt.Errorf("skills.yaml: duplicate code %q", s.Code)
		}
		if _, ok := f.Groups[s.Group]; !ok {
			return nil, fmt.Errorf("skills.yaml: skill %q has unknown group %q", s.Code, s.Group)
		}
		for _, sp := range s.Specs {
			if !IsSpecialization(sp) {
				return nil, fmt.Errorf("skills.yaml: skill %q has unknown spec %q", s.Code, sp)
			}
		}
		if s.Topic != "" && !isAnyTopic(s.Topic) {
			return nil, fmt.Errorf("skills.yaml: skill %q has unknown topic %q", s.Code, s.Topic)
		}
		idx.byCode[s.Code] = s
		for _, alias := range append([]string{s.Name}, s.Synonyms...) {
			key := NormalizeTerm(alias)
			if key == "" {
				continue
			}
			if prev, dup := idx.byAlias[key]; dup && prev != s {
				return nil, fmt.Errorf("skills.yaml: alias %q belongs to both %q and %q", alias, prev.Code, s.Code)
			}
			idx.byAlias[key] = s
		}
	}
	return idx, nil
}

func isAnyTopic(code string) bool {
	for spec := range Topics {
		if IsTopic(spec, code) {
			return true
		}
	}
	return false
}

// ByCode возвращает навык по коду.
func (x *SkillIndex) ByCode(code string) (*Skill, bool) {
	s, ok := x.byCode[code]
	return s, ok
}

// Lookup находит навык по названию или синониму (после нормализации).
func (x *SkillIndex) Lookup(term string) (*Skill, bool) {
	s, ok := x.byAlias[NormalizeTerm(term)]
	return s, ok
}

// Aliases возвращает все нормализованные алиасы (для построения матчера по тексту),
// отсортированные по убыванию длины — длинные совпадения проверяются первыми.
func (x *SkillIndex) Aliases() []string {
	out := make([]string, 0, len(x.byAlias))
	for a := range x.byAlias {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

// Len — число навыков.
func (x *SkillIndex) Len() int { return len(x.Skills) }

// NormalizeTerm приводит термин к ключу сравнения: нижний регистр, ё→е,
// пробелы и дефисы схлопываются, крайние знаки препинания убираются.
func NormalizeTerm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case r == '+' || r == '#' || r == '.':
			// c++, c#, node.js — значимые символы
			b.WriteRune(r)
			lastSpace = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !lastSpace && b.Len() > 0 {
				b.WriteByte(' ')
				lastSpace = true
			}
		default:
			// прочие знаки препинания отбрасываем
		}
	}
	return strings.TrimRight(strings.TrimSpace(b.String()), ".")
}

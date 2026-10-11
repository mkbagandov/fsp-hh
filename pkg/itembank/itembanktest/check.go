// Package itembanktest — проверка инвариантов семейства заданий (правила скилла
// item-family-authoring). Каждое семейство обязано иметь тест вида
//
//	func TestFamily_X(t *testing.T) { itembanktest.CheckFamily(t, X{}) }
package itembanktest

import (
	"encoding/json"
	"strings"
	"testing"

	"fsp/pkg/itembank"
)

// Seeds — сколько seed проверяется; MinUnique — минимум различимых экземпляров.
const (
	Seeds     = 200
	MinUnique = 150
)

// CheckFamily проверяет паспорт, детерминированность, вариативность, корректность
// вариантов и ответов семейства.
func CheckFamily(t *testing.T, f itembank.Family) {
	t.Helper()
	m := f.Meta()
	if err := m.Validate(); err != nil {
		t.Fatalf("%s: meta: %v", m.ID, err)
	}
	unique := map[string]struct{}{}
	for seed := uint64(1); seed <= Seeds; seed++ {
		in, err := f.Generate(seed)
		if err != nil {
			t.Fatalf("%s: seed %d: generate: %v", m.ID, seed, err)
		}
		if in.FamilyID != m.ID {
			t.Fatalf("%s: seed %d: instance family id %q", m.ID, seed, in.FamilyID)
		}
		if in.Seed != seed {
			t.Fatalf("%s: seed %d: instance seed %d", m.ID, seed, in.Seed)
		}
		if err := in.Validate(m.Format); err != nil {
			t.Fatalf("%s: seed %d: instance: %v", m.ID, seed, err)
		}
		again, err := f.Generate(seed)
		if err != nil {
			t.Fatalf("%s: seed %d: regenerate: %v", m.ID, seed, err)
		}
		// Сравниваем через JSON: указатели (Code) сериализуются по значению, а не по адресу.
		first, _ := json.Marshal(in)
		second, _ := json.Marshal(again)
		if string(first) != string(second) {
			t.Fatalf("%s: seed %d: generation is not deterministic", m.ID, seed)
		}
		key := in.Stem
		if in.Code != nil {
			key += "\x00" + in.Code.Source
		}
		for _, o := range in.Options {
			key += "\x00" + o.Text
		}
		unique[key] = struct{}{}

		// Верный ответ принимается, первый дистрактор — нет.
		if !itembank.Check(m.Format, in.Answer, correctSubmission(in)) {
			t.Fatalf("%s: seed %d: canonical answer is rejected by Check", m.ID, seed)
		}
		if wrong, ok := wrongSubmission(in, m.Format); ok && itembank.Check(m.Format, in.Answer, wrong) {
			t.Fatalf("%s: seed %d: a wrong submission is accepted by Check", m.ID, seed)
		}
		if strings.Contains(in.Stem, "TODO") || strings.Contains(in.Explanation, "TODO") {
			t.Fatalf("%s: seed %d: TODO left in text", m.ID, seed)
		}
	}
	if len(unique) < MinUnique {
		t.Fatalf("%s: only %d unique instances across %d seeds (want >= %d)", m.ID, len(unique), Seeds, MinUnique)
	}
}

func correctSubmission(in itembank.Instance) itembank.Submission {
	n := in.Answer.Number
	return itembank.Submission{
		Keys: in.Answer.Keys, Number: &n, Text: in.Answer.Text, Order: in.Answer.Order, Rows: in.Answer.Rows,
	}
}

func wrongSubmission(in itembank.Instance, format itembank.Format) (itembank.Submission, bool) {
	switch format {
	case itembank.FormatSingleChoice:
		for _, o := range in.Options {
			if o.Key != in.Answer.Keys[0] {
				return itembank.Submission{Keys: []string{o.Key}}, true
			}
		}
	case itembank.FormatMultiChoice:
		for _, o := range in.Options {
			found := false
			for _, k := range in.Answer.Keys {
				if k == o.Key {
					found = true
				}
			}
			if !found {
				return itembank.Submission{Keys: append(append([]string{}, in.Answer.Keys...), o.Key)}, true
			}
		}
	case itembank.FormatNumeric:
		n := in.Answer.Number + in.Answer.Tolerance*2 + 1
		return itembank.Submission{Number: &n}, true
	case itembank.FormatShortText, itembank.FormatCodeOutput:
		return itembank.Submission{Text: in.Answer.Text + " x"}, true
	case itembank.FormatOrdering:
		if len(in.Answer.Order) >= 2 {
			o := append([]string{}, in.Answer.Order...)
			o[0], o[1] = o[1], o[0]
			return itembank.Submission{Order: o}, true
		}
	case itembank.FormatSQLResult:
		rows := append([][]string{}, in.Answer.Rows...)
		rows = append(rows, []string{"extra"})
		return itembank.Submission{Rows: rows}, true
	}
	return itembank.Submission{}, false
}

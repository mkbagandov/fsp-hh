package all

import (
	"testing"

	"fsp/pkg/itembank"
	"fsp/pkg/itembank/itembanktest"
)

// TestAllFamilies — сквозная проверка реестра: каждое семейство проходит инварианты,
// идентификаторы уникальны, у каждой специализации есть задания.
func TestAllFamilies(t *testing.T) {
	fams := itembank.All()
	if len(fams) < 15 {
		t.Fatalf("expected at least 15 families, got %d", len(fams))
	}
	bySpec := map[itembank.Spec]int{}
	for _, f := range fams {
		f := f
		t.Run(f.Meta().ID, func(t *testing.T) { itembanktest.CheckFamily(t, f) })
		bySpec[f.Meta().Spec]++
	}
	// У каждой специализации — собственный банк не меньше порога теста (DefaultMinSpecFamilies в сервисе тестирования).
	for _, spec := range []itembank.Spec{itembank.SpecCommon, itembank.SpecBackend, itembank.SpecFrontend, itembank.SpecDataML, itembank.SpecDevOps, itembank.SpecQA, itembank.SpecSecurity} {
		if bySpec[spec] < 10 {
			t.Errorf("спец. %s: %d семейств, нужно не меньше 10", spec, bySpec[spec])
		}
	}
	t.Logf("families: %d, by spec: %v", len(fams), bySpec)
}

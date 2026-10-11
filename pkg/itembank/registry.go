package itembank

import (
	"fmt"
	"sort"
	"sync"
)

var (
	regMu    sync.RWMutex
	families = map[string]Family{}
)

// Register добавляет семейство в реестр. Паникует при дубликате или невалидном паспорте:
// это ошибка программиста, которую должен поймать init() при старте и тесты.
func Register(f Family) {
	m := f.Meta()
	if err := m.Validate(); err != nil {
		panic(fmt.Sprintf("itembank: family %q: %v", m.ID, err))
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := families[m.ID]; dup {
		panic(fmt.Sprintf("itembank: duplicate family id %q", m.ID))
	}
	families[m.ID] = f
}

// Get возвращает семейство по идентификатору.
func Get(id string) (Family, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	f, ok := families[id]
	return f, ok
}

// All возвращает все семейства, отсортированные по ID.
func All() []Family {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Family, 0, len(families))
	for _, f := range families {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta().ID < out[j].Meta().ID })
	return out
}

// BySpec возвращает семейства специализации вместе с общим блоком common.
func BySpec(spec Spec) []Family {
	all := All()
	out := make([]Family, 0, len(all))
	for _, f := range all {
		if s := f.Meta().Spec; s == spec || s == SpecCommon {
			out = append(out, f)
		}
	}
	return out
}

// Count — число зарегистрированных семейств.
func Count() int {
	regMu.RLock()
	defer regMu.RUnlock()
	return len(families)
}

// Reset очищает реестр (только для тестов).
func Reset() {
	regMu.Lock()
	defer regMu.Unlock()
	families = map[string]Family{}
}

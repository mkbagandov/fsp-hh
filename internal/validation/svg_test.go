package validation

import (
	"slices"
	"strings"
	"testing"
)

func TestNiceTicks(t *testing.T) {
	cases := []struct {
		lo, hi float64
		target int
		want   []float64
	}{
		{0, 0.98, 5, []float64{0, 0.2, 0.4, 0.6, 0.8, 1}},
		{0, 1.05, 5, []float64{0, 0.25, 0.5, 0.75, 1, 1.25}},
		{0, 40, 6, []float64{0, 10, 20, 30, 40}},
		{-3, 3, 6, []float64{-3, -2, -1, 0, 1, 2, 3}},
		{0, 13.4, 5, []float64{0, 2.5, 5, 7.5, 10, 12.5, 15}},
	}
	for _, c := range cases {
		if got := niceTicks(c.lo, c.hi, c.target); !slices.Equal(got, c.want) {
			t.Errorf("niceTicks(%v, %v, %d) = %v, want %v", c.lo, c.hi, c.target, got, c.want)
		}
	}
}

func TestNumberFormat(t *testing.T) {
	for v, want := range map[float64]string{0.25: "0,25", -1: "−1", 2.5: "2,5", 0.30000000000000004: "0,3", -0.0000001: "0"} {
		if got := fmtTick(v); got != want {
			t.Errorf("fmtTick(%v) = %q, want %q", v, got, want)
		}
	}
	if got := fmtNum(0.836); got != "0,84" {
		t.Errorf("fmtNum = %q", got)
	}
}

func TestWrapLabels(t *testing.T) {
	got := wrap("Линейный тест (равномерно по сложности)", 19, 3)
	if !slices.Equal(got, []string{"Линейный тест", "(равномерно по", "сложности)"}) {
		t.Errorf("wrap = %q", got)
	}
	got = wrap("один два три четыре пять шесть", 6, 2)
	if len(got) != 2 || !strings.HasSuffix(got[1], "…") {
		t.Errorf("переполнение должно обрезаться многоточием: %q", got)
	}
}

func TestBarChartLabelsFitGroups(t *testing.T) {
	labels := []string{"CAT (остановка по SE ≤ 0,30)", "CAT фиксированной длины", "Линейный тест (равномерно по сложности)", "Случайные задания"}
	svg := BarChart("CAT против линейных форм", "значение", labels, []Series{{Name: "RMSE оценки θ", Values: []float64{0.38, 0.38, 0.44, 0.43}}, {Name: "грейд точно", Values: []float64{0.86, 0.86, 0.82, 0.79}}}, 0)
	if strings.Contains(svg, "…") {
		t.Error("подписи методов должны помещаться без обрезки")
	}
	for _, want := range []string{">1<", ">0,86<", ">Линейный тест<"} {
		if !strings.Contains(svg, want) {
			t.Errorf("нет %q в графике", want)
		}
	}
}

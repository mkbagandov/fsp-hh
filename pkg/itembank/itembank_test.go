package itembank

import (
	"testing"
	"time"
)

func TestCheckFormats(t *testing.T) {
	one := 42.0
	cases := []struct {
		name   string
		format Format
		ans    Answer
		sub    Submission
		want   bool
	}{
		{"single ok", FormatSingleChoice, Answer{Keys: []string{"b"}}, Submission{Keys: []string{"b"}}, true},
		{"single wrong", FormatSingleChoice, Answer{Keys: []string{"b"}}, Submission{Keys: []string{"a"}}, false},
		{"single two keys", FormatSingleChoice, Answer{Keys: []string{"b"}}, Submission{Keys: []string{"a", "b"}}, false},
		{"multi ok any order", FormatMultiChoice, Answer{Keys: []string{"a", "c"}}, Submission{Keys: []string{"c", "a"}}, true},
		{"multi partial", FormatMultiChoice, Answer{Keys: []string{"a", "c"}}, Submission{Keys: []string{"a"}}, false},
		{"numeric exact", FormatNumeric, Answer{Number: 42}, Submission{Number: &one}, true},
		{"numeric tolerance", FormatNumeric, Answer{Number: 42.004, Tolerance: 0.01}, Submission{Number: &one}, true},
		{"numeric nil", FormatNumeric, Answer{Number: 42}, Submission{}, false},
		{"text normalize", FormatShortText, Answer{Text: "hello  world"}, Submission{Text: "  hello world \n"}, true},
		{"text case", FormatCodeOutput, Answer{Text: "True"}, Submission{Text: "true"}, false},
		{"code multiline", FormatCodeOutput, Answer{Text: "1\n2\n3"}, Submission{Text: "1\r\n2\r\n3\r\n\r\n"}, true},
		{"ordering ok", FormatOrdering, Answer{Order: []string{"c", "a", "b"}}, Submission{Order: []string{"c", "a", "b"}}, true},
		{"ordering wrong", FormatOrdering, Answer{Order: []string{"c", "a", "b"}}, Submission{Order: []string{"a", "c", "b"}}, false},
		{"sql ordered", FormatSQLResult, Answer{Rows: [][]string{{"alice", "2"}, {"bob", "1"}}}, Submission{Rows: [][]string{{"Alice", "2"}, {"bob", "1"}}}, true},
		{"sql wrong order", FormatSQLResult, Answer{Rows: [][]string{{"alice", "2"}, {"bob", "1"}}}, Submission{Rows: [][]string{{"bob", "1"}, {"alice", "2"}}}, false},
		{"sql unordered", FormatSQLResult, Answer{Rows: [][]string{{"alice", "2"}, {"bob", "1"}}, OrderInsensitive: true}, Submission{Rows: [][]string{{"bob", "1"}, {"alice", "2"}}}, true},
		{"sql empty", FormatSQLResult, Answer{Rows: [][]string{}}, Submission{Rows: [][]string{}}, true},
	}
	for _, c := range cases {
		if got := Check(c.format, c.ans, c.sub); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestRandHelpersDeterministic(t *testing.T) {
	r1, r2 := NewRand(7, 1), NewRand(7, 1)
	if Ident(r1, "table") != Ident(r2, "table") || IntBetween(r1, 1, 100) != IntBetween(r2, 1, 100) {
		t.Fatal("same seed must give same sequence")
	}
	r3 := NewRand(7, 2)
	diff := false
	for i := 0; i < 10; i++ {
		if IntBetween(r1, 1, 1000) != IntBetween(r3, 1, 1000) {
			diff = true
		}
	}
	if !diff {
		t.Fatal("different salt must give different sequence")
	}
	opts, key := ShuffledOptions(NewRand(1, 1), "x", "y", "x", "z", "")
	if len(opts) != 3 || key == "" {
		t.Fatalf("shuffled options: %+v %q", opts, key)
	}
	found := false
	for _, o := range opts {
		if o.Key == key && o.Text == "x" {
			found = true
		}
	}
	if !found {
		t.Fatal("correct key must point at the correct text")
	}
	ord, order := OrderingOptions(NewRand(3, 3), []string{"first", "second", "third", "fourth"})
	if len(ord) != 4 || len(order) != 4 {
		t.Fatal("ordering options")
	}
	for i, k := range order {
		for _, o := range ord {
			if o.Key == k && o.Text != []string{"first", "second", "third", "fourth"}[i] {
				t.Fatalf("order key %q maps to wrong text", k)
			}
		}
	}
	if len(PickN(NewRand(5, 5), []int{1, 2, 3}, 5)) != 3 {
		t.Fatal("PickN must clamp n")
	}
}

type dummy struct{ id string }

func (d dummy) Meta() Meta {
	return Meta{ID: d.id, Spec: SpecBackend, Topic: "sql", Grade: GradeJunior, Format: FormatNumeric, IRT: IRTParams{A: 1, B: 0, C: 0}, TimeLimit: time.Minute}
}
func (dummy) Generate(uint64) (Instance, error) { return Instance{}, nil }

func TestRegistry(t *testing.T) {
	Reset()
	defer Reset()
	Register(dummy{"backend.sql.a.v1"})
	Register(dummy{"backend.sql.b.v1"})
	if Count() != 2 || len(BySpec(SpecBackend)) != 2 || len(BySpec(SpecFrontend)) != 0 {
		t.Fatal("registry counts")
	}
	if _, ok := Get("backend.sql.a.v1"); !ok {
		t.Fatal("get")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate must panic")
		}
	}()
	Register(dummy{"backend.sql.a.v1"})
}

func TestMetaValidate(t *testing.T) {
	bad := Meta{ID: "Backend.sql.x", Spec: "x", Grade: "y", Format: "z", IRT: IRTParams{A: 9, B: 9, C: 9}, TimeLimit: time.Second}
	if err := bad.Validate(); err == nil {
		t.Fatal("bad meta must fail")
	}
	good := Meta{ID: "common.sql.join-count.v2", Spec: SpecCommon, Topic: "sql", Grade: GradeMiddle, Format: FormatNumeric, IRT: IRTParams{A: 1.2, B: 0.3, C: 0}, TimeLimit: 90 * time.Second}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
}

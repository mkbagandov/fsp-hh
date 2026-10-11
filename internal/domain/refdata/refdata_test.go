package refdata

import (
	"testing"
)

func TestSpecializationsAndGrades(t *testing.T) {
	if !IsSpecialization("backend") || !IsSelectableSpecialization("backend") {
		t.Fatal("backend must be a selectable specialization")
	}
	if IsSelectableSpecialization("common") {
		t.Fatal("common must not be selectable")
	}
	if IsSpecialization("fullstack") {
		t.Fatal("unknown specialization accepted")
	}
	if GradeOrder("junior") >= GradeOrder("middle") || GradeOrder("middle") >= GradeOrder("senior") {
		t.Fatal("grade order must be strict")
	}
	if g, ok := NextGrade("senior"); ok {
		t.Fatalf("senior must not have next grade, got %v", g)
	}
	if g, ok := NextGrade("junior"); !ok || g.Code != "middle" {
		t.Fatalf("next of junior must be middle, got %v %v", g, ok)
	}
	if g, ok := PrevGrade("middle"); !ok || g.Code != "junior" {
		t.Fatalf("prev of middle must be junior, got %v %v", g, ok)
	}
}

func TestTopics(t *testing.T) {
	for spec := range Topics {
		if !IsSpecialization(spec) {
			t.Fatalf("topics defined for unknown specialization %q", spec)
		}
	}
	if !IsTopic("backend", "sql") || !IsTopic("backend", "git") {
		t.Fatal("backend must see own topics and common topics")
	}
	if IsTopic("backend", "react_state") {
		t.Fatal("frontend topic must not be visible to backend")
	}
	if len(AllTopicCodes()) < 40 {
		t.Fatalf("expected at least 40 topics, got %d", len(AllTopicCodes()))
	}
}

func TestRelevance(t *testing.T) {
	if Relevance("algorithmic", "backend") != 1.0 {
		t.Fatal("algorithmic→backend must be 1.0")
	}
	if Relevance("uav", "frontend") != MinRelevance {
		t.Fatal("low relevance must be clamped to minimum")
	}
	if Relevance("unknown", "backend") != MinRelevance {
		t.Fatal("unknown discipline must yield minimum relevance")
	}
	if LevelWeight("international") != 1.0 || LevelWeight("nope") != 0 {
		t.Fatal("level weights")
	}
	if ResultWeight("place", 1) != 1.0 || ResultWeight("place", 7) != 0.6 || ResultWeight("place", 40) != 0.3 {
		t.Fatal("result weights by place")
	}
}

func TestNormalizeTerm(t *testing.T) {
	cases := map[string]string{
		"  PostgreSQL ": "postgresql",
		"Node.js":       "node.js",
		"C++":           "c++",
		"C#":            "c#",
		"Ёлка-Палка":    "елка палка",
		"react/redux":   "react redux",
		"k8s,":          "k8s",
	}
	for in, want := range cases {
		if got := NormalizeTerm(in); got != want {
			t.Errorf("NormalizeTerm(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkillsYAML(t *testing.T) {
	idx, err := Skills()
	if err != nil {
		t.Fatalf("skills.yaml invalid: %v", err)
	}
	if idx.Len() < 250 {
		t.Fatalf("expected at least 250 skills, got %d", idx.Len())
	}
	for _, probe := range []string{"PostgreSQL", "postgres", "Go", "golang", "React", "Kubernetes", "k8s", "SQL"} {
		if _, ok := idx.Lookup(probe); !ok {
			t.Errorf("lookup %q failed", probe)
		}
	}
	for _, s := range idx.Skills {
		if len(s.Specs) == 0 {
			t.Errorf("skill %q has no specs", s.Code)
		}
	}
}

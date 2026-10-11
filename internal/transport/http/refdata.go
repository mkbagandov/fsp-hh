package httptransport

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	apptesting "fsp/internal/app/testing"
	"fsp/internal/domain/refdata"
)

type refItem struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type refdataOutput struct {
	Body struct {
		Specializations []refItem `json:"specializations" doc:"Выбираемые при регистрации"`
		// Тест доступен только там, где есть собственный банк заданий (не меньше 10 семейств без общего блока).
		TestableSpecializations []string             `json:"testable_specializations" doc:"Специализации, по которым доступен тест"`
		Grades                  []refItem            `json:"grades"`
		WorkFormats             []refItem            `json:"work_formats"`
		DeclineReasons          []refItem            `json:"decline_reasons"`
		FSPDisciplines          []refItem            `json:"fsp_disciplines"`
		FSPLevels               []refItem            `json:"fsp_levels"`
		SkillGroups             []refItem            `json:"skill_groups"`
		Topics                  map[string][]refItem `json:"topics" doc:"Темы заданий по специализации (common — общий блок)"`
	}
}

type skillItem struct {
	Code  string   `json:"code"`
	Name  string   `json:"name"`
	Group string   `json:"group"`
	Specs []string `json:"specs"`
}

type skillsOutput struct {
	Body struct {
		Items []skillItem `json:"items"`
		Total int         `json:"total"`
	}
}

type skillsQuery struct {
	Q     string `query:"q" maxLength:"64" doc:"Подстрока названия или синонима"`
	Spec  string `query:"spec" enum:"backend,frontend,data_ml,devops,qa,security,common" doc:"Фильтр по специализации"`
	Limit int    `query:"limit" minimum:"1" maximum:"500" default:"100"`
}

// registerRefdata — публичные справочники для форм фронтенда.
func registerRefdata(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "refdata-get", Method: http.MethodGet, Path: "/refdata",
		Summary: "Справочники", Tags: []string{"refdata"},
		Description: "Специализации, грейды, форматы работы, причины отказа, дисциплины и уровни ФСП, группы навыков, темы заданий.",
	}, func(_ context.Context, _ *struct{}) (*refdataOutput, error) {
		out := &refdataOutput{}
		for _, s := range refdata.Specializations {
			if s.Selectable {
				out.Body.Specializations = append(out.Body.Specializations, refItem{s.Code, s.Name})
			}
		}
		out.Body.TestableSpecializations = apptesting.TestableSpecializations(apptesting.DefaultMinSpecFamilies)
		for _, g := range refdata.Grades {
			out.Body.Grades = append(out.Body.Grades, refItem{g.Code, g.Name})
		}
		for _, w := range refdata.WorkFormats {
			out.Body.WorkFormats = append(out.Body.WorkFormats, refItem{w.Code, w.Name})
		}
		for _, r := range refdata.DeclineReasons {
			out.Body.DeclineReasons = append(out.Body.DeclineReasons, refItem{r.Code, r.Name})
		}
		for _, d := range refdata.Disciplines {
			out.Body.FSPDisciplines = append(out.Body.FSPDisciplines, refItem{d.Code, d.Name})
		}
		for _, l := range refdata.Levels {
			out.Body.FSPLevels = append(out.Body.FSPLevels, refItem{l.Code, l.Name})
		}
		idx := refdata.MustSkills()
		for code, name := range idx.Groups {
			out.Body.SkillGroups = append(out.Body.SkillGroups, refItem{code, name})
		}
		sort.Slice(out.Body.SkillGroups, func(i, j int) bool { return out.Body.SkillGroups[i].Code < out.Body.SkillGroups[j].Code })
		out.Body.Topics = map[string][]refItem{}
		for spec, topics := range refdata.Topics {
			for _, t := range topics {
				out.Body.Topics[spec] = append(out.Body.Topics[spec], refItem{t.Code, t.Name})
			}
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "skills-list", Method: http.MethodGet, Path: "/skills",
		Summary: "Справочник навыков", Tags: []string{"refdata"},
		Description: "Поиск по названию и синонимам; используется автодополнением в анкете и потребности.",
	}, func(_ context.Context, in *skillsQuery) (*skillsOutput, error) {
		idx := refdata.MustSkills()
		q := refdata.NormalizeTerm(in.Q)
		out := &skillsOutput{}
		out.Body.Items = []skillItem{}
		limit := in.Limit
		if limit <= 0 {
			limit = 100
		}
		for _, s := range idx.Skills {
			if in.Spec != "" && !contains(s.Specs, in.Spec) && !contains(s.Specs, "common") {
				continue
			}
			if q != "" {
				match := strings.Contains(refdata.NormalizeTerm(s.Name), q)
				for _, syn := range s.Synonyms {
					if match {
						break
					}
					match = strings.Contains(refdata.NormalizeTerm(syn), q)
				}
				if !match {
					continue
				}
			}
			out.Body.Items = append(out.Body.Items, skillItem{Code: s.Code, Name: s.Name, Group: s.Group, Specs: s.Specs})
			if len(out.Body.Items) >= limit {
				break
			}
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

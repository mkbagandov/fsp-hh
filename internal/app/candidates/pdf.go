package candidates

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"fsp/internal/app/fsp"
	"fsp/internal/domain/refdata"
	"fsp/internal/infrastructure/pdfgen"
)

// PDFDeps — зависимости генерации PDF-профиля.
type PDFDeps struct {
	Achievements *fsp.Syncer
	VerifyBase   string // например https://host/verify
	FSPLinked    func(ctx context.Context, userID uuid.UUID) (bool, error)
}

// SetPDFDeps включает генерацию PDF-профиля.
func (s *Service) SetPDFDeps(d PDFDeps) { s.pdf = &d }

// ProfilePDF строит PDF-профиль. includeContacts=false — версия для работодателя без гранта.
func (s *Service) ProfilePDF(ctx context.Context, userID uuid.UUID, includeContacts bool) ([]byte, error) {
	prof, err := s.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	data := pdfgen.Profile{
		FullName:     strings.TrimSpace(strings.Join([]string{prof.LastName, prof.FirstName, deref(prof.MiddleName)}, " ")),
		Headline:     deref(prof.Headline),
		City:         deref(prof.City),
		Relocation:   prof.Relocation,
		About:        deref(prof.About),
		GeneratedAt:  time.Now(),
		NoFSPHistory: true,
	}
	if prof.ExperienceYears != nil {
		data.ExperienceYears = *prof.ExperienceYears
	}
	if prof.SalaryExpected != nil {
		data.SalaryExpected = *prof.SalaryExpected
	}
	for _, f := range prof.WorkFormats {
		for _, wf := range refdata.WorkFormats {
			if wf.Code == f {
				data.WorkFormats = append(data.WorkFormats, strings.ToLower(wf.Name))
			}
		}
	}
	if prof.PrimarySpecialization != nil {
		if sp, ok := refdata.SpecializationByCode(*prof.PrimarySpecialization); ok {
			data.Specialization = sp.Name
		}
	}
	for _, sk := range prof.Skills {
		data.Skills = append(data.Skills, pdfgen.Skill{Name: sk.Name, Confirmed: sk.Source == "test_confirmed"})
	}
	for _, e := range prof.Experiences {
		period := formatPeriod(e.StartedOn, e.EndedOn)
		data.Experiences = append(data.Experiences, pdfgen.Experience{
			Period: period, Company: e.CompanyName, Position: e.Position, Description: deref(e.Description), Stack: e.Stack,
		})
	}
	for _, e := range prof.Education {
		line := e.Institution
		if e.Field != nil {
			line += ", " + *e.Field
		}
		if e.Degree != nil {
			line += " (" + *e.Degree + ")"
		}
		if e.GraduatedYear != nil {
			line += fmt.Sprintf(", %d", *e.GraduatedYear)
		}
		data.Education = append(data.Education, line)
	}
	if includeContacts {
		c, err := s.GetContacts(ctx, userID)
		if err == nil {
			if c.Phone != nil {
				data.Contacts = append(data.Contacts, *c.Phone)
			}
			if c.Email != nil {
				data.Contacts = append(data.Contacts, *c.Email)
			}
			if c.Telegram != nil {
				data.Contacts = append(data.Contacts, "@"+*c.Telegram)
			}
		}
	}
	if s.pdf != nil {
		if s.pdf.FSPLinked != nil {
			if linked, err := s.pdf.FSPLinked(ctx, userID); err == nil && linked {
				data.NoFSPHistory = false
			}
		}
		if s.pdf.Achievements != nil {
			items, err := s.pdf.Achievements.List(ctx, userID)
			if err == nil {
				for _, a := range items {
					data.Achievements = append(data.Achievements, formatAchievement(a))
				}
				if len(items) > 0 {
					data.NoFSPHistory = false
				}
			}
		}
		if s.pdf.VerifyBase != "" {
			data.VerifyURL = strings.TrimRight(s.pdf.VerifyBase, "/") + "/" + userID.String()
		}
	}
	return pdfgen.Build(data)
}

func formatPeriod(started string, ended *string) string {
	from := started
	if t, err := time.Parse("2006-01-02", started); err == nil {
		from = t.Format("01.2006")
	}
	to := "настоящее время"
	if ended != nil {
		to = *ended
		if t, err := time.Parse("2006-01-02", *ended); err == nil {
			to = t.Format("01.2006")
		}
	}
	return from + " — " + to
}

func formatAchievement(a fsp.Achievement) string {
	level := a.Level
	for _, l := range refdata.Levels {
		if l.Code == a.Level {
			level = strings.ToLower(l.Name)
		}
	}
	result := ""
	switch a.ResultKind {
	case "place":
		if a.Place != nil {
			result = fmt.Sprintf("%d место", *a.Place)
		}
	case "rank":
		if a.RankTitle != nil {
			result = *a.RankTitle
		}
	case "rating":
		if a.Rating != nil {
			result = fmt.Sprintf("рейтинг %d", *a.Rating)
		}
	default:
		result = "участие"
	}
	s := fmt.Sprintf("%s (%s, %d)", a.EventName, level, a.EventDate.Year())
	if result != "" {
		s += " — " + result
	}
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

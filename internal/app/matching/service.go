package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/internal/domain"
	"fsp/internal/domain/refdata"
	"fsp/internal/storage/postgres"
	"fsp/internal/storage/postgres/sqlcgen"
)

// Service — подбор поверх БД: загрузка кандидатов, версии подборок, карта, карточка, поиск.
type Service struct {
	pool    *pgxpool.Pool
	q       *sqlcgen.Queries
	weights Weights
	log     *slog.Logger
	now     func() time.Time
}

// New создаёт сервис.
func New(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{pool: pool, q: sqlcgen.New(pool), weights: DefaultWeights(), log: log, now: time.Now}
}

// Weights — текущие веса (для отображения «как считается»).
func (s *Service) Weights() Weights { return s.weights }

// NeedFromRow переводит потребность из БД в модель подбора.
func NeedFromRow(n *sqlcgen.Need) Need {
	formats := make([]string, 0, len(n.WorkFormats))
	for _, f := range n.WorkFormats {
		formats = append(formats, string(f))
	}
	city := ""
	if n.City != nil {
		city = *n.City
	}
	return Need{
		CompanyID: n.CompanyID, Specialization: string(n.Specialization), GradeMin: string(n.GradeMin), GradeMax: string(n.GradeMax),
		MustSkills: n.MustSkillIds, NiceSkills: n.NiceSkillIds,
		Salary: domain.SalaryRange{From: int64(n.SalaryFrom), To: int64(n.SalaryTo)}, WorkFormats: formats, City: city, RemoteOK: n.RemoteOk,
	}
}

// loadCandidates собирает кандидатов категорий с навыками, достижениями и историей приглашений компании.
func (s *Service) loadCandidates(ctx context.Context, spec string, grades []string, companyID uuid.UUID) ([]Candidate, error) {
	gradeCodes := make([]sqlcgen.GradeCode, 0, len(grades))
	for _, g := range grades {
		gradeCodes = append(gradeCodes, sqlcgen.GradeCode(g))
	}
	rows, err := s.q.ListMatchingCandidates(ctx, sqlcgen.ListMatchingCandidatesParams{Specialization: sqlcgen.SpecializationCode(spec), Grades: gradeCodes})
	if err != nil {
		return nil, fmt.Errorf("list candidates: %w", err)
	}
	if len(rows) == 0 {
		return []Candidate{}, nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	byID := map[uuid.UUID]*Candidate{}
	out := make([]Candidate, 0, len(rows))
	for _, r := range rows {
		c := Candidate{
			UserID: r.UserID, FirstName: r.FirstName, LastName: r.LastName, Grade: string(r.Grade), GradeStatus: string(r.GradeStatus),
			CategoryUpdatedAt: r.CategoryUpdatedAt, Relocation: r.Relocation, Visibility: string(r.Visibility),
			ShowFullName: r.ShowFullName, ShowCity: r.ShowCity, ShowSalary: r.ShowSalaryExpected, ShowFSP: r.ShowFsp, FSPLinked: r.FspLinked,
			ShowExperience: r.ShowExperience, Skills: map[int32]string{}, SkillNames: map[int32]string{},
		}
		if r.Theta != nil {
			c.Theta, c.HasTheta = *r.Theta, true
		}
		if r.Se != nil {
			c.SE = *r.Se
		}
		if r.Percentile != nil {
			c.Percentile = *r.Percentile
		}
		// Скрытые кандидатом поля в подбор не попадают: иначе их выдали бы объяснения, фильтры
		// (?city=…) или агрегаты карты категории. Минимум зарплаты приватен по замыслу и в тексты не выводится.
		if r.City != nil && r.ShowCity {
			c.City = *r.City
		}
		if r.Headline != nil {
			c.Headline = *r.Headline
		}
		if r.SalaryMin != nil {
			c.SalaryMin = int64(*r.SalaryMin)
		}
		if r.SalaryExpected != nil {
			if r.ShowSalaryExpected {
				c.SalaryExpected = int64(*r.SalaryExpected)
			} else {
				c.SalaryHidden = true
			}
		}
		if r.ExperienceYears != nil && r.ShowExperience {
			c.ExperienceYears = *r.ExperienceYears
		}
		for _, f := range r.WorkFormats {
			c.WorkFormats = append(c.WorkFormats, string(f))
		}
		out = append(out, c)
		ids = append(ids, r.UserID)
	}
	for i := range out {
		byID[out[i].UserID] = &out[i]
	}
	skills, err := s.q.ListCandidateSkillsForUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, sk := range skills {
		if c, ok := byID[sk.UserID]; ok {
			c.Skills[sk.SkillID] = string(sk.Source)
			c.SkillNames[sk.SkillID] = sk.Name
		}
	}
	achs, err := s.q.ListFSPAchievementsForUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, a := range achs {
		if c, ok := byID[a.UserID]; ok && c.ShowFSP {
			place := 0
			if a.Place != nil {
				place = int(*a.Place)
			}
			c.Achievements = append(c.Achievements, Achievement{Discipline: string(a.Discipline), Level: string(a.Level), ResultKind: string(a.ResultKind), Place: place, EventDate: a.EventDate, EventName: a.EventName})
		}
	}
	if companyID != uuid.Nil {
		hist, err := s.q.ListInvitationHistoryForCompany(ctx, sqlcgen.ListInvitationHistoryForCompanyParams{CompanyID: companyID, UserIds: ids})
		if err != nil {
			return nil, err
		}
		for _, h := range hist {
			if c, ok := byID[h.CandidateUserID]; ok {
				switch h.Status {
				case sqlcgen.InvitationStatusDeclined:
					c.DeclinedCompany++
				case sqlcgen.InvitationStatusAccepted:
					c.AcceptedAny++
				}
			}
		}
	}
	return out, nil
}

// CategoryMapFor строит карту категорий для потребности (все грейды специализации).
func (s *Service) CategoryMapFor(ctx context.Context, need Need) ([]CategoryStat, error) {
	all := make([]string, 0, len(refdata.Grades))
	for _, g := range refdata.Grades {
		all = append(all, g.Code)
	}
	cands, err := s.loadCandidates(ctx, need.Specialization, all, need.CompanyID)
	if err != nil {
		return nil, err
	}
	return CategoryMap(need, cands), nil
}

// CandidateSummary — карточка в подборке (без контактов, с учётом приватности).
type CandidateSummary struct {
	ID              uuid.UUID `json:"id"`
	DisplayName     string    `json:"display_name"`
	Headline        string    `json:"headline,omitempty"`
	Grade           string    `json:"grade"`
	GradeStatus     string    `json:"grade_status"`
	Percentile      *float64  `json:"percentile,omitempty"`
	City            string    `json:"city,omitempty"`
	SalaryExpected  *int64    `json:"salary_expected,omitempty"`
	WorkFormats     []string  `json:"work_formats"`
	ExperienceYears *float64  `json:"experience_years,omitempty"`
	SkillsMatched   int       `json:"skills_matched"`
	HasFSP          bool      `json:"has_fsp"`
	Invited         bool      `json:"invited" doc:"Есть активное или завершённое приглашение от компании"`
}

// SelectionItem — строка подборки.
type SelectionItem struct {
	Rank       int                `json:"rank"`
	Candidate  CandidateSummary   `json:"candidate"`
	Score      float64            `json:"score"`
	Relevance  float64            `json:"relevance"`
	PAccept    float64            `json:"p_accept"`
	Components map[string]float64 `json:"components"`
	Facts      []ExplanationFact  `json:"facts"`
	Confirmed  bool               `json:"confirmed"`
}

// SelectionVersion — версия подборки.
type SelectionVersion struct {
	VersionNo   int              `json:"version_no"`
	CreatedAt   time.Time        `json:"created_at"`
	Filters     SelectionFilters `json:"filters"`
	Weights     Weights          `json:"weights"`
	CategoryMap []CategoryStat   `json:"category_map"`
	ItemsCount  int              `json:"items_count"`
	Items       []SelectionItem  `json:"items"`
	Diff        *SelectionDiff   `json:"diff,omitempty" doc:"Изменения относительно предыдущей версии"`
}

// SelectionVersionSummary — краткая запись в списке версий.
type SelectionVersionSummary struct {
	VersionNo  int              `json:"version_no"`
	CreatedAt  time.Time        `json:"created_at"`
	ItemsCount int              `json:"items_count"`
	Filters    SelectionFilters `json:"filters"`
}

// BuildVersion считает подборку под потребность и сохраняет её как новую версию.
func (s *Service) BuildVersion(ctx context.Context, needRow *sqlcgen.Need, createdBy uuid.UUID, f SelectionFilters) (SelectionVersion, error) {
	need := NeedFromRow(needRow)
	cands, err := s.loadCandidates(ctx, need.Specialization, need.Grades(), need.CompanyID)
	if err != nil {
		return SelectionVersion{}, err
	}
	if err := s.withNeedSkillNames(ctx, need, cands); err != nil {
		return SelectionVersion{}, err
	}
	now := s.now()
	ranked := Rank(need, cands, s.weights, now)
	var skillIDs []int32
	if len(f.SkillCodes) > 0 {
		rows, err := s.q.GetSkillsByCodes(ctx, f.SkillCodes)
		if err != nil {
			return SelectionVersion{}, err
		}
		for _, r := range rows {
			skillIDs = append(skillIDs, r.ID)
		}
		if len(skillIDs) != len(f.SkillCodes) {
			return SelectionVersion{}, domain.Validation("skills", "skill_unknown", "В фильтре есть неизвестные навыки")
		}
	}
	invited, err := s.invitedSet(ctx, need.CompanyID, needRow.ID)
	if err != nil {
		return SelectionVersion{}, err
	}
	filtered := Apply(ranked, f, skillIDs, invited)
	all := make([]string, 0, len(refdata.Grades))
	for _, g := range refdata.Grades {
		all = append(all, g.Code)
	}
	allCands := cands
	if len(need.Grades()) != len(refdata.Grades) {
		if allCands, err = s.loadCandidates(ctx, need.Specialization, all, need.CompanyID); err != nil {
			return SelectionVersion{}, err
		}
	}
	cmap := CategoryMap(need, allCands)

	curIDs := make([]uuid.UUID, 0, len(filtered))
	for _, sc := range filtered {
		curIDs = append(curIDs, sc.Candidate.UserID)
	}
	fJSON, _ := json.Marshal(f)
	var prevIDs []uuid.UUID
	var version SelectionVersion
	reuse := false
	err = postgres.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		sel, err := q.GetOrCreateSelection(ctx, needRow.ID)
		if err != nil {
			return err
		}
		// Строки кандидатов блокируются от удаления до конца транзакции; исчезнувшие между чтением
		// и записью отбрасываются — снимок версии не ссылается на несуществующих пользователей.
		if filtered, err = keepExisting(ctx, tx, filtered); err != nil {
			return err
		}
		curIDs = curIDs[:0]
		for _, sc := range filtered {
			curIDs = append(curIDs, sc.Candidate.UserID)
		}
		if prev, err := q.GetLatestSelectionVersion(ctx, sel.ID); err == nil {
			items, err := q.ListSelectionItems(ctx, prev.ID)
			if err != nil {
				return err
			}
			for _, it := range items {
				prevIDs = append(prevIDs, it.CandidateUserID)
			}
			// Те же фильтры и тот же состав в том же порядке — новая версия ничего не добавит:
			// повторные нажатия «Собрать» не плодят снимки в базе.
			var pf SelectionFilters
			if json.Unmarshal(prev.Filters, &pf) == nil {
				if pJSON, _ := json.Marshal(pf); string(pJSON) == string(fJSON) && slices.Equal(prevIDs, curIDs) {
					reuse = true
					return nil
				}
			}
		}
		no, err := q.NextSelectionVersionNo(ctx, sel.ID)
		if err != nil {
			return err
		}
		wJSON, _ := json.Marshal(s.weights)
		mJSON, _ := json.Marshal(cmap)
		row, err := q.CreateSelectionVersion(ctx, sqlcgen.CreateSelectionVersionParams{
			SelectionID: sel.ID, VersionNo: int32(no), Filters: fJSON, Weights: wJSON, CategoryMap: mJSON, //nolint:gosec // номера версий малы
			ItemsCount: int32(len(filtered)), CreatedBy: createdBy, //nolint:gosec // ≤ limit
		})
		if err != nil {
			return err
		}
		params := make([]sqlcgen.InsertSelectionItemsParams, 0, len(filtered))
		for i, sc := range filtered {
			comp, _ := json.Marshal(sc.Components)
			expl, _ := json.Marshal(sc.Facts)
			params = append(params, sqlcgen.InsertSelectionItemsParams{
				VersionID: row.ID, CandidateUserID: sc.Candidate.UserID, Rank: int32(i + 1), Score: sc.Score, Relevance: sc.Relevance, //nolint:gosec // ≤ limit
				PAccept: sc.PAccept, Components: comp, Explanation: expl, GradeStatus: sqlcgen.GradeStatus(sc.Candidate.GradeStatus),
			})
		}
		if len(params) > 0 {
			if _, err := q.InsertSelectionItems(ctx, params); err != nil {
				return err
			}
		}
		version = SelectionVersion{VersionNo: int(no), CreatedAt: row.CreatedAt, Filters: f, Weights: s.weights, CategoryMap: cmap, ItemsCount: len(filtered)}
		return nil
	})
	if err != nil {
		return SelectionVersion{}, err
	}
	if reuse {
		return s.GetVersion(ctx, needRow, 0)
	}
	for _, sc := range filtered {
		version.Items = append(version.Items, toItem(len(version.Items)+1, sc, need, invited))
	}
	if version.Items == nil {
		version.Items = []SelectionItem{}
	}
	if prevIDs != nil {
		d := DiffVersions(prevIDs, curIDs)
		version.Diff = &d
	}
	return version, nil
}

// GetVersion читает сохранённую версию (0 — последняя).
func (s *Service) GetVersion(ctx context.Context, needRow *sqlcgen.Need, versionNo int) (SelectionVersion, error) {
	sel, err := s.q.GetOrCreateSelection(ctx, needRow.ID)
	if err != nil {
		return SelectionVersion{}, err
	}
	var row *sqlcgen.SelectionVersion
	if versionNo <= 0 {
		row, err = s.q.GetLatestSelectionVersion(ctx, sel.ID)
	} else {
		row, err = s.q.GetSelectionVersion(ctx, sqlcgen.GetSelectionVersionParams{SelectionID: sel.ID, VersionNo: int32(versionNo)}) //nolint:gosec // малые номера
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return SelectionVersion{}, domain.NotFound("selection_version")
	}
	if err != nil {
		return SelectionVersion{}, err
	}
	items, err := s.q.ListSelectionItems(ctx, row.ID)
	if err != nil {
		return SelectionVersion{}, err
	}
	need := NeedFromRow(needRow)
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.CandidateUserID)
	}
	// Карточки — по текущему состоянию профилей; объяснения и баллы — из снимка версии.
	all := make([]string, 0, len(refdata.Grades))
	for _, g := range refdata.Grades {
		all = append(all, g.Code)
	}
	cands, err := s.loadCandidates(ctx, need.Specialization, all, need.CompanyID)
	if err != nil {
		return SelectionVersion{}, err
	}
	byID := map[uuid.UUID]Candidate{}
	for _, c := range cands {
		byID[c.UserID] = c
	}
	invited, err := s.invitedSet(ctx, need.CompanyID, needRow.ID)
	if err != nil {
		return SelectionVersion{}, err
	}
	v := SelectionVersion{VersionNo: int(row.VersionNo), CreatedAt: row.CreatedAt, Weights: s.weights, ItemsCount: int(row.ItemsCount), Items: []SelectionItem{}}
	_ = json.Unmarshal(row.Filters, &v.Filters)
	_ = json.Unmarshal(row.Weights, &v.Weights)
	_ = json.Unmarshal(row.CategoryMap, &v.CategoryMap)
	for _, it := range items {
		sc := Scored{Score: it.Score, Relevance: it.Relevance, PAccept: it.PAccept, Confirmed: it.GradeStatus == sqlcgen.GradeStatusConfirmed || it.GradeStatus == sqlcgen.GradeStatusUpgradeSuggested}
		_ = json.Unmarshal(it.Components, &sc.Components)
		_ = json.Unmarshal(it.Explanation, &sc.Facts)
		c, ok := byID[it.CandidateUserID]
		if !ok {
			// Кандидат с тех пор скрыл профиль, отозвал согласие или удалил аккаунт: сохранённые
			// объяснения могли содержать его данные — не показываем их.
			c = Candidate{UserID: it.CandidateUserID, FirstName: "Кандидат", GradeStatus: string(it.GradeStatus), Skills: map[int32]string{}}
			c.Headline = "профиль скрыт или удалён"
			sc.Facts = []ExplanationFact{}
		} else {
			sc.Facts = redactFacts(sc.Facts, c)
		}
		sc.Candidate = c
		v.Items = append(v.Items, toItem(int(it.Rank), sc, need, invited))
	}
	if row.VersionNo > 1 {
		if prev, err := s.q.GetSelectionVersion(ctx, sqlcgen.GetSelectionVersionParams{SelectionID: sel.ID, VersionNo: row.VersionNo - 1}); err == nil {
			prevItems, _ := s.q.ListSelectionItems(ctx, prev.ID)
			prevIDs := make([]uuid.UUID, 0, len(prevItems))
			for _, it := range prevItems {
				prevIDs = append(prevIDs, it.CandidateUserID)
			}
			d := DiffVersions(prevIDs, ids)
			v.Diff = &d
		}
	}
	return v, nil
}

// ListVersions — версии подборки потребности.
func (s *Service) ListVersions(ctx context.Context, needID uuid.UUID) ([]SelectionVersionSummary, error) {
	sel, err := s.q.GetOrCreateSelection(ctx, needID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListSelectionVersions(ctx, sel.ID)
	if err != nil {
		return nil, err
	}
	out := make([]SelectionVersionSummary, 0, len(rows))
	for _, r := range rows {
		vs := SelectionVersionSummary{VersionNo: int(r.VersionNo), CreatedAt: r.CreatedAt, ItemsCount: int(r.ItemsCount)}
		_ = json.Unmarshal(r.Filters, &vs.Filters)
		out = append(out, vs)
	}
	return out, nil
}

// SearchInput — поиск по банку без потребности.
type SearchInput struct {
	Specialization string
	Grades         []string
	SkillCodes     []string
	City           string
	Formats        []string
	HasFSP         *bool
	GradeStatuses  []string
	Limit          int
}

// Search ранжирует кандидатов по синтетической потребности из фильтров (без вилки — s_fit нейтрален).
func (s *Service) Search(ctx context.Context, companyID uuid.UUID, in SearchInput) ([]SelectionItem, error) {
	if !refdata.IsSelectableSpecialization(in.Specialization) {
		return nil, domain.Validation("specialization", "specialization_invalid", "Укажите специализацию")
	}
	grades := in.Grades
	if len(grades) == 0 {
		for _, g := range refdata.Grades {
			grades = append(grades, g.Code)
		}
	}
	for _, g := range grades {
		if !refdata.IsGrade(g) {
			return nil, domain.Validation("grades", "grade_invalid", "Неизвестный грейд: "+g)
		}
	}
	var skillIDs []int32
	if len(in.SkillCodes) > 0 {
		rows, err := s.q.GetSkillsByCodes(ctx, in.SkillCodes)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			skillIDs = append(skillIDs, r.ID)
		}
	}
	need := Need{CompanyID: companyID, Specialization: in.Specialization, GradeMin: grades[0], GradeMax: grades[len(grades)-1],
		MustSkills: skillIDs, Salary: domain.SalaryRange{From: 1, To: domain.MaxSalary}, RemoteOK: true, WorkFormats: in.Formats, City: in.City}
	cands, err := s.loadCandidates(ctx, in.Specialization, grades, companyID)
	if err != nil {
		return nil, err
	}
	if err := s.withNeedSkillNames(ctx, need, cands); err != nil {
		return nil, err
	}
	ranked := Rank(need, cands, s.weights, s.now())
	invited, err := s.invitedSet(ctx, companyID, uuid.Nil)
	if err != nil {
		return nil, err
	}
	filtered := Apply(ranked, SelectionFilters{Grades: grades, GradeStatuses: in.GradeStatuses, City: in.City, Formats: in.Formats, HasFSP: in.HasFSP, Limit: in.Limit}, skillIDs, invited)
	out := make([]SelectionItem, 0, len(filtered))
	for i, sc := range filtered {
		out = append(out, toItem(i+1, sc, need, invited))
	}
	return out, nil
}

func (s *Service) invitedSet(ctx context.Context, companyID, needID uuid.UUID) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	if companyID == uuid.Nil {
		return out, nil
	}
	var nid uuid.NullUUID
	if needID != uuid.Nil {
		nid = uuid.NullUUID{UUID: needID, Valid: true}
	}
	rows, err := s.q.ListInvitationsByCompany(ctx, sqlcgen.ListInvitationsByCompanyParams{CompanyID: companyID, NeedID: nid})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.CandidateUserID] = true
	}
	return out, nil
}

func toItem(rank int, sc Scored, need Need, invited map[uuid.UUID]bool) SelectionItem {
	c := sc.Candidate
	sum := CandidateSummary{ID: c.UserID, Grade: c.Grade, GradeStatus: c.GradeStatus, Headline: c.Headline, WorkFormats: c.WorkFormats, HasFSP: len(c.Achievements) > 0, Invited: invited[c.UserID]}
	if sum.WorkFormats == nil {
		sum.WorkFormats = []string{}
	}
	if c.ShowFullName {
		sum.DisplayName = c.LastName + " " + c.FirstName
	} else {
		sum.DisplayName = c.FirstName
	}
	if c.HasTheta {
		p := c.Percentile
		sum.Percentile = &p
	}
	if c.ShowCity {
		sum.City = c.City
	}
	if c.ShowSalary && c.SalaryExpected > 0 {
		v := c.SalaryExpected
		sum.SalaryExpected = &v
	}
	if c.ExperienceYears > 0 {
		v := c.ExperienceYears
		sum.ExperienceYears = &v
	}
	for _, id := range append(append([]int32{}, need.MustSkills...), need.NiceSkills...) {
		if _, ok := c.Skills[id]; ok {
			sum.SkillsMatched++
		}
	}
	return SelectionItem{Rank: rank, Candidate: sum, Score: sc.Score, Relevance: sc.Relevance, PAccept: sc.PAccept, Components: sc.Components, Facts: sc.Facts, Confirmed: sc.Confirmed}
}

// CandidateCardView — карточка кандидата для работодателя (с учётом приватности и гранта).
type CandidateCardView struct {
	ID               uuid.UUID         `json:"id"`
	DisplayName      string            `json:"display_name"`
	Headline         string            `json:"headline,omitempty"`
	About            string            `json:"about,omitempty"`
	City             string            `json:"city,omitempty"`
	WorkFormats      []string          `json:"work_formats"`
	Relocation       bool              `json:"relocation"`
	SalaryExpected   *int64            `json:"salary_expected,omitempty"`
	ExperienceYears  *float64          `json:"experience_years,omitempty"`
	Specialization   string            `json:"specialization,omitempty"`
	Grade            string            `json:"grade,omitempty"`
	GradeStatus      string            `json:"grade_status,omitempty"`
	Percentile       *float64          `json:"percentile,omitempty"`
	ConfirmedAt      *time.Time        `json:"confirmed_at,omitempty"`
	Skills           []CardSkill       `json:"skills"`
	Experiences      []CardExperience  `json:"experiences"`
	Achievements     []CardAchievement `json:"achievements"`
	FSPState         string            `json:"fsp_state" enum:"none,linked_empty,has_achievements,hidden" doc:"none — нет данных ФСП"`
	ContactsGranted  bool              `json:"contacts_granted"`
	Contacts         *CardContacts     `json:"contacts,omitempty" doc:"Только при активном гранте"`
	InvitationStatus string            `json:"invitation_status,omitempty" doc:"Статус последнего приглашения компании"`
}

// CardSkill — навык в карточке.
type CardSkill struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

// CardExperience — опыт в карточке.
type CardExperience struct {
	CompanyName string  `json:"company_name"`
	Position    string  `json:"position"`
	StartedOn   string  `json:"started_on"`
	EndedOn     *string `json:"ended_on,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CardAchievement — достижение ФСП в карточке.
type CardAchievement struct {
	Discipline string `json:"discipline"`
	Level      string `json:"level"`
	EventName  string `json:"event_name"`
	Year       int    `json:"year"`
	Result     string `json:"result"`
}

// CardContacts — контакты (по гранту).
type CardContacts struct {
	Phone    *string           `json:"phone,omitempty"`
	Telegram *string           `json:"telegram,omitempty"`
	Email    *string           `json:"email,omitempty"`
	Other    map[string]string `json:"other,omitempty"`
}

// CandidateCard возвращает карточку с учётом приватности; контакты — только при гранте.
func (s *Service) CandidateCard(ctx context.Context, companyID, candidateID uuid.UUID) (CandidateCardView, error) {
	r, err := s.q.GetCandidateCard(ctx, candidateID)
	if errors.Is(err, pgx.ErrNoRows) && companyID != uuid.Nil {
		// Скрытый кандидат виден компании, которой он сам открыл контакты (отклик или принятое приглашение).
		if _, gerr := s.q.GetActiveContactGrant(ctx, sqlcgen.GetActiveContactGrantParams{CandidateUserID: candidateID, CompanyID: companyID}); gerr == nil {
			if anyRow, aerr := s.q.GetCandidateCardAny(ctx, candidateID); aerr == nil {
				row := sqlcgen.GetCandidateCardRow(*anyRow)
				r, err = &row, nil
			}
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return CandidateCardView{}, domain.NotFound("candidate")
	}
	if err != nil {
		return CandidateCardView{}, err
	}
	card := CandidateCardView{ID: candidateID, Relocation: r.Relocation, Skills: []CardSkill{}, Experiences: []CardExperience{}, Achievements: []CardAchievement{}, WorkFormats: []string{}}
	if r.ShowFullName {
		card.DisplayName = r.LastName + " " + r.FirstName
	} else {
		card.DisplayName = r.FirstName
	}
	if r.Headline != nil {
		card.Headline = *r.Headline
	}
	if r.About != nil {
		card.About = *r.About
	}
	if r.ShowCity && r.City != nil {
		card.City = *r.City
	}
	for _, f := range r.WorkFormats {
		card.WorkFormats = append(card.WorkFormats, string(f))
	}
	if r.ShowSalaryExpected && r.SalaryExpected != nil {
		v := int64(*r.SalaryExpected)
		card.SalaryExpected = &v
	}
	if r.ShowExperience {
		card.ExperienceYears = r.ExperienceYears
	}
	if r.Specialization != nil {
		card.Specialization = string(*r.Specialization)
	}
	if r.Grade != nil {
		card.Grade = string(*r.Grade)
	}
	if r.GradeStatus != nil {
		card.GradeStatus = string(*r.GradeStatus)
	}
	card.Percentile = r.Percentile
	card.ConfirmedAt = r.ConfirmedAt
	skills, err := s.q.ListCandidateSkills(ctx, candidateID)
	if err != nil {
		return CandidateCardView{}, err
	}
	for _, sk := range skills {
		card.Skills = append(card.Skills, CardSkill{Code: sk.Code, Name: sk.Name, Source: string(sk.Source)})
	}
	if r.ShowExperience {
		exps, err := s.q.ListCandidateExperiences(ctx, candidateID)
		if err != nil {
			return CandidateCardView{}, err
		}
		for _, e := range exps {
			ce := CardExperience{CompanyName: e.CompanyName, Position: e.Position, StartedOn: e.StartedOn.Format("2006-01-02"), Description: e.Description}
			if e.EndedOn != nil {
				v := e.EndedOn.Format("2006-01-02")
				ce.EndedOn = &v
			}
			card.Experiences = append(card.Experiences, ce)
		}
	}
	card.FSPState = "none"
	if _, err := s.q.GetFSPLinkByUser(ctx, candidateID); err == nil {
		card.FSPState = "linked_empty"
		if !r.ShowFsp {
			card.FSPState = "hidden"
		} else {
			achs, err := s.q.ListFSPAchievements(ctx, candidateID)
			if err != nil {
				return CandidateCardView{}, err
			}
			for _, a := range achs {
				res := "участие"
				switch a.ResultKind {
				case sqlcgen.FspResultKindPlace:
					if a.Place != nil {
						res = fmt.Sprintf("%d место", *a.Place)
					}
				case sqlcgen.FspResultKindRank:
					if a.RankTitle != nil {
						res = *a.RankTitle
					}
				case sqlcgen.FspResultKindRating:
					if a.Rating != nil {
						res = fmt.Sprintf("рейтинг %d", *a.Rating)
					}
				}
				card.Achievements = append(card.Achievements, CardAchievement{Discipline: string(a.Discipline), Level: string(a.Level), EventName: a.EventName, Year: a.EventDate.Year(), Result: res})
			}
			if len(card.Achievements) > 0 {
				card.FSPState = "has_achievements"
			}
		}
	}
	if companyID != uuid.Nil {
		switch contacts, err := s.Contacts(ctx, companyID, candidateID); {
		case err == nil:
			card.ContactsGranted = true
			card.Contacts = &contacts
		case !domain.Is(err, domain.KindForbidden):
			return CandidateCardView{}, err
		}
		if rows, err := s.q.ListInvitationHistoryForCompany(ctx, sqlcgen.ListInvitationHistoryForCompanyParams{CompanyID: companyID, UserIds: []uuid.UUID{candidateID}}); err == nil && len(rows) > 0 {
			card.InvitationStatus = string(rows[0].Status)
		}
	}
	return card, nil
}

// Contacts возвращает контакты кандидата только при активном гранте компании (принятое приглашение);
// иначе 403 contacts_not_granted. Каждый просмотр пишется в аудит.
func (s *Service) Contacts(ctx context.Context, companyID, candidateID uuid.UUID) (CardContacts, error) {
	row, err := s.q.ContactsForCompany(ctx, sqlcgen.ContactsForCompanyParams{UserID: candidateID, CompanyID: companyID})
	if errors.Is(err, pgx.ErrNoRows) {
		return CardContacts{}, domain.Forbidden("contacts_not_granted", "Контакты доступны после того, как кандидат примет приглашение")
	}
	if err != nil {
		return CardContacts{}, err
	}
	cc := CardContacts{Phone: row.Phone, Telegram: row.Telegram, Email: row.Email, Other: map[string]string{}}
	_ = json.Unmarshal(row.Other, &cc.Other)
	var actor uuid.NullUUID
	if p, ok := domain.PrincipalFrom(ctx); ok {
		actor = uuid.NullUUID{UUID: p.UserID, Valid: true}
	}
	_ = s.q.InsertAudit(ctx, sqlcgen.InsertAuditParams{
		ActorUserID: actor, Action: "contacts.view", ObjectType: "candidate",
		ObjectID: uuid.NullUUID{UUID: candidateID, Valid: true}, Details: json.RawMessage(fmt.Sprintf(`{"company_id":%q}`, companyID)),
	})
	return cc, nil
}

// Preview ранжирует кандидатов под потребность без сохранения версии (подборка под вакансию).
func (s *Service) Preview(ctx context.Context, need Need, f SelectionFilters) ([]SelectionItem, error) {
	cands, err := s.loadCandidates(ctx, need.Specialization, need.Grades(), need.CompanyID)
	if err != nil {
		return nil, err
	}
	if err := s.withNeedSkillNames(ctx, need, cands); err != nil {
		return nil, err
	}
	var skillIDs []int32
	if len(f.SkillCodes) > 0 {
		rows, err := s.q.GetSkillsByCodes(ctx, f.SkillCodes)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			skillIDs = append(skillIDs, r.ID)
		}
	}
	invited, err := s.invitedSet(ctx, need.CompanyID, uuid.Nil)
	if err != nil {
		return nil, err
	}
	filtered := Apply(Rank(need, cands, s.weights, s.now()), f, skillIDs, invited)
	out := make([]SelectionItem, 0, len(filtered))
	for i, sc := range filtered {
		out = append(out, toItem(i+1, sc, need, invited))
	}
	return out, nil
}

// withNeedSkillNames дописывает кандидатам названия навыков потребности, которых нет в их профиле:
// факт «Нет в профиле: …» должен называть навыки словами, а не идентификаторами.
func (s *Service) withNeedSkillNames(ctx context.Context, need Need, cands []Candidate) error {
	ids := append(append([]int32{}, need.MustSkills...), need.NiceSkills...)
	if len(ids) == 0 || len(cands) == 0 {
		return nil
	}
	rows, err := s.q.GetSkillsByIDs(ctx, ids)
	if err != nil {
		return err
	}
	for i := range cands {
		if cands[i].SkillNames == nil {
			cands[i].SkillNames = map[int32]string{}
		}
		for _, r := range rows {
			if _, ok := cands[i].SkillNames[r.ID]; !ok {
				cands[i].SkillNames[r.ID] = r.Name
			}
		}
	}
	return nil
}

// keepExisting оставляет кандидатов, чьи учётные записи существуют, и удерживает их строки
// блокировкой FOR KEY SHARE до конца транзакции (удаление пользователя дождётся записи снимка).
func keepExisting(ctx context.Context, tx pgx.Tx, scored []Scored) ([]Scored, error) {
	if len(scored) == 0 {
		return scored, nil
	}
	ids := make([]uuid.UUID, 0, len(scored))
	for _, sc := range scored {
		ids = append(ids, sc.Candidate.UserID)
	}
	rows, err := tx.Query(ctx, `SELECT id FROM users WHERE id = ANY($1) FOR KEY SHARE`, ids)
	if err != nil {
		return nil, err
	}
	alive, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	if len(alive) == len(scored) {
		return scored, nil
	}
	set := make(map[uuid.UUID]bool, len(alive))
	for _, id := range alive {
		set[id] = true
	}
	kept := make([]Scored, 0, len(alive))
	for _, sc := range scored {
		if set[sc.Candidate.UserID] {
			kept = append(kept, sc)
		}
	}
	return kept, nil
}

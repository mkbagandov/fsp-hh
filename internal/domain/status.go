package domain

// Перечисления домена и допустимые переходы. Значения совпадают с enum в PostgreSQL
// (см. docs/02-data-model.md). Переход проверяется методом CanTransitionTo —
// единственное место, где закодированы правила из docs/02, раздел 4.

// Role — роль пользователя.
type Role string

const (
	RoleCandidate Role = "candidate"
	RoleEmployer  Role = "employer"
	RoleAdmin     Role = "admin"
)

// Valid сообщает, допустимо ли значение.
func (r Role) Valid() bool { return r == RoleCandidate || r == RoleEmployer || r == RoleAdmin }

// Visibility — режим видимости кандидата для работодателей.
type Visibility string

const (
	VisibilityOpen    Visibility = "open"
	VisibilityPassive Visibility = "passive"
	VisibilityHidden  Visibility = "hidden"
)

// Valid сообщает, допустимо ли значение.
func (v Visibility) Valid() bool {
	return v == VisibilityOpen || v == VisibilityPassive || v == VisibilityHidden
}

// SkillSource — откуда взялся навык в профиле.
type SkillSource string

const (
	SkillDeclared      SkillSource = "declared"
	SkillFromResume    SkillSource = "resume"
	SkillTestConfirmed SkillSource = "test_confirmed"
)

// Weight — вес навыка в расчёте совпадения: подтверждённый тестом — 1.0, остальные — 0.5.
func (s SkillSource) Weight() float64 {
	if s == SkillTestConfirmed {
		return 1.0
	}
	return 0.5
}

// ConsentKind — вид согласия.
type ConsentKind string

const (
	ConsentProcessing  ConsentKind = "processing"
	ConsentPublication ConsentKind = "publication"
	ConsentFSPData     ConsentKind = "fsp_data"
)

// GradeStatus — статус подтверждения грейда.
type GradeStatus string

const (
	GradeClaimed          GradeStatus = "claimed"
	GradeConfirmed        GradeStatus = "confirmed"
	GradeNotConfirmed     GradeStatus = "not_confirmed"
	GradeUpgradeSuggested GradeStatus = "upgrade_suggested"
)

// Valid сообщает, допустимо ли значение.
func (s GradeStatus) Valid() bool {
	switch s {
	case GradeClaimed, GradeConfirmed, GradeNotConfirmed, GradeUpgradeSuggested:
		return true
	}
	return false
}

// InvitationStatus — статус приглашения.
type InvitationStatus string

const (
	InvitationSent      InvitationStatus = "sent"
	InvitationViewed    InvitationStatus = "viewed"
	InvitationAccepted  InvitationStatus = "accepted"
	InvitationDeclined  InvitationStatus = "declined"
	InvitationWithdrawn InvitationStatus = "withdrawn"
	InvitationExpired   InvitationStatus = "expired"
)

var invitationTransitions = map[InvitationStatus][]InvitationStatus{
	InvitationSent:   {InvitationViewed, InvitationWithdrawn, InvitationExpired},
	InvitationViewed: {InvitationAccepted, InvitationDeclined, InvitationWithdrawn, InvitationExpired},
}

// CanTransitionTo проверяет допустимость перехода.
func (s InvitationStatus) CanTransitionTo(t InvitationStatus) bool {
	return contains(invitationTransitions[s], t)
}

// IsFinal — терминальный ли статус.
func (s InvitationStatus) IsFinal() bool { return len(invitationTransitions[s]) == 0 }

// IsActive — ждёт ли приглашение ответа.
func (s InvitationStatus) IsActive() bool { return s == InvitationSent || s == InvitationViewed }

// ApplicationStatus — статус отклика.
type ApplicationStatus string

const (
	ApplicationSent      ApplicationStatus = "sent"
	ApplicationViewed    ApplicationStatus = "viewed"
	ApplicationInvited   ApplicationStatus = "invited"
	ApplicationRejected  ApplicationStatus = "rejected"
	ApplicationWithdrawn ApplicationStatus = "withdrawn"
)

var applicationTransitions = map[ApplicationStatus][]ApplicationStatus{
	ApplicationSent:   {ApplicationViewed, ApplicationWithdrawn},
	ApplicationViewed: {ApplicationInvited, ApplicationRejected, ApplicationWithdrawn},
}

// CanTransitionTo проверяет допустимость перехода.
func (s ApplicationStatus) CanTransitionTo(t ApplicationStatus) bool {
	return contains(applicationTransitions[s], t)
}

// VacancyStatus — статус вакансии.
type VacancyStatus string

const (
	VacancyDraft     VacancyStatus = "draft"
	VacancyPublished VacancyStatus = "published"
	VacancyClosed    VacancyStatus = "closed"
)

var vacancyTransitions = map[VacancyStatus][]VacancyStatus{
	VacancyDraft:     {VacancyPublished},
	VacancyPublished: {VacancyDraft, VacancyClosed},
}

// CanTransitionTo проверяет допустимость перехода.
func (s VacancyStatus) CanTransitionTo(t VacancyStatus) bool {
	return contains(vacancyTransitions[s], t)
}

// NeedStatus — статус потребности.
type NeedStatus string

const (
	NeedDraft    NeedStatus = "draft"
	NeedActive   NeedStatus = "active"
	NeedArchived NeedStatus = "archived"
)

var needTransitions = map[NeedStatus][]NeedStatus{
	NeedDraft:    {NeedActive, NeedArchived},
	NeedActive:   {NeedArchived, NeedDraft},
	NeedArchived: {NeedActive},
}

// CanTransitionTo проверяет допустимость перехода.
func (s NeedStatus) CanTransitionTo(t NeedStatus) bool { return contains(needTransitions[s], t) }

// TestSessionStatus — статус тест-сессии.
type TestSessionStatus string

const (
	TestCreated    TestSessionStatus = "created"
	TestInProgress TestSessionStatus = "in_progress"
	TestCompleted  TestSessionStatus = "completed"
	TestAbandoned  TestSessionStatus = "abandoned"
	TestExpired    TestSessionStatus = "expired"
)

var testSessionTransitions = map[TestSessionStatus][]TestSessionStatus{
	TestCreated:    {TestInProgress, TestAbandoned, TestExpired},
	TestInProgress: {TestCompleted, TestAbandoned, TestExpired},
}

// CanTransitionTo проверяет допустимость перехода.
func (s TestSessionStatus) CanTransitionTo(t TestSessionStatus) bool {
	return contains(testSessionTransitions[s], t)
}

// IsOpen — можно ли продолжать сессию.
func (s TestSessionStatus) IsOpen() bool { return s == TestCreated || s == TestInProgress }

// MicroTaskAssignmentStatus — статус назначенного микро-задания.
type MicroTaskAssignmentStatus string

const (
	AssignmentOffered   MicroTaskAssignmentStatus = "offered"
	AssignmentAccepted  MicroTaskAssignmentStatus = "accepted"
	AssignmentSubmitted MicroTaskAssignmentStatus = "submitted"
	AssignmentReviewed  MicroTaskAssignmentStatus = "reviewed"
	AssignmentDeclined  MicroTaskAssignmentStatus = "declined"
	AssignmentExpired   MicroTaskAssignmentStatus = "expired"
)

var assignmentTransitions = map[MicroTaskAssignmentStatus][]MicroTaskAssignmentStatus{
	AssignmentOffered:   {AssignmentAccepted, AssignmentDeclined, AssignmentExpired},
	AssignmentAccepted:  {AssignmentSubmitted, AssignmentExpired},
	AssignmentSubmitted: {AssignmentReviewed},
}

// CanTransitionTo проверяет допустимость перехода.
func (s MicroTaskAssignmentStatus) CanTransitionTo(t MicroTaskAssignmentStatus) bool {
	return contains(assignmentTransitions[s], t)
}

// ContactGrantSource — основание раскрытия контактов.
type ContactGrantSource string

const (
	GrantByInvitation  ContactGrantSource = "invitation_accepted"
	GrantByApplication ContactGrantSource = "application_sent"
)

func contains[T comparable](xs []T, x T) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

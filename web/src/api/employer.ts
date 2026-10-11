// Данные работодателя: компания, потребности, карта категорий, подборки, кандидаты, приглашения, вакансии, отклики.
import { useQuery } from "@tanstack/react-query";
import { api, unwrap, isApiError, type Schemas } from "./client";

export type Company = Schemas["Company"];
export type Need = Schemas["Need"];
export type NeedBody = Schemas["NeedBody"];
export type Extraction = Schemas["Extraction"];
export type CategoryStat = Schemas["CategoryStat"];
export type Weights = Schemas["Weights"];
export type SelectionVersion = Schemas["SelectionVersion"];
export type SelectionItem = Schemas["SelectionItem"];
export type SelectionFilters = Schemas["SelectionFilters"];
export type ExplanationFact = Schemas["ExplanationFact"];
export type CandidateSummary = Schemas["CandidateSummary"];
export type CandidateCard = Schemas["CandidateCardView"];
export type Feedback = Schemas["Feedback"];
export type Invitation = Schemas["Invitation"];
export type Vacancy = Schemas["Vacancy"];
export type VacancyBody = Schemas["VacancyBody"];
export type Application = Schemas["Application"];
export type ContactMethod = Schemas["ContactMethod"];

export const ek = {
  company: ["employer", "company"] as const,
  needs: ["employer", "needs"] as const,
  need: (id: string) => ["employer", "needs", id] as const,
  map: (id: string) => ["employer", "needs", id, "map"] as const,
  selection: (id: string, v: number) => ["employer", "needs", id, "selection", v] as const,
  versions: (id: string) => ["employer", "needs", id, "versions"] as const,
  feedback: (id: string) => ["employer", "needs", id, "feedback"] as const,
  invitations: (needId?: string) => ["employer", "invitations", needId ?? "all"] as const,
  vacancies: ["employer", "vacancies"] as const,
  vacancy: (id: string) => ["employer", "vacancies", id] as const,
  applications: (id: string) => ["employer", "vacancies", id, "applications"] as const,
  card: (id: string) => ["employer", "candidates", id] as const,
  search: (p: unknown) => ["employer", "candidates", "search", p] as const,
};

export function useCompany() {
  return useQuery({
    queryKey: ek.company,
    queryFn: async () => {
      try {
        return unwrap(await api.GET("/employer/company"));
      } catch (e) {
        if (isApiError(e) && e.status === 404) return null;
        throw e;
      }
    },
  });
}
export function useNeeds() {
  return useQuery({ queryKey: ek.needs, queryFn: async () => unwrap(await api.GET("/employer/needs")).items ?? [] });
}
export function useNeed(id: string) {
  return useQuery({ queryKey: ek.need(id), enabled: !!id, queryFn: async () => unwrap(await api.GET("/employer/needs/{id}", { params: { path: { id } } })) });
}
export function useNeedMap(id: string) {
  return useQuery({ queryKey: ek.map(id), enabled: !!id, queryFn: async () => unwrap(await api.GET("/employer/needs/{id}/map", { params: { path: { id } } })) });
}
export function useSelection(id: string, version = 0) {
  return useQuery({
    queryKey: ek.selection(id, version),
    enabled: !!id,
    retry: false,
    queryFn: async () => {
      try {
        return unwrap(await api.GET("/employer/needs/{id}/selection", { params: { path: { id }, query: { version } } }));
      } catch (e) {
        if (isApiError(e) && e.status === 404) return null;
        throw e;
      }
    },
  });
}
export function useVersions(id: string) {
  return useQuery({ queryKey: ek.versions(id), enabled: !!id, queryFn: async () => unwrap(await api.GET("/employer/needs/{id}/selection/versions", { params: { path: { id } } })).items ?? [] });
}
export function useFeedback(id: string) {
  return useQuery({ queryKey: ek.feedback(id), enabled: !!id, queryFn: async () => unwrap(await api.GET("/employer/needs/{id}/feedback", { params: { path: { id } } })) });
}
export function useEmployerInvitations(needId?: string) {
  return useQuery({
    queryKey: ek.invitations(needId),
    queryFn: async () => unwrap(await api.GET("/employer/invitations", { params: { query: needId ? { need_id: needId } : {} } })).items ?? [],
  });
}
export function useVacancies() {
  return useQuery({ queryKey: ek.vacancies, queryFn: async () => unwrap(await api.GET("/employer/vacancies")).items ?? [] });
}
export function useVacancy(id: string) {
  return useQuery({ queryKey: ek.vacancy(id), enabled: !!id, queryFn: async () => unwrap(await api.GET("/employer/vacancies/{id}", { params: { path: { id } } })) });
}
export function useVacancyApplications(id: string) {
  return useQuery({ queryKey: ek.applications(id), enabled: !!id, queryFn: async () => unwrap(await api.GET("/employer/vacancies/{id}/applications", { params: { path: { id } } })).items ?? [] });
}
export function useCandidateCard(id: string) {
  return useQuery({ queryKey: ek.card(id), enabled: !!id, queryFn: async () => unwrap(await api.GET("/employer/candidates/{id}", { params: { path: { id } } })) });
}

export type SearchParams = { specialization: string; grades?: string[]; skills?: string[]; city?: string; formats?: string[]; has_fsp?: "" | "true" | "false"; grade_statuses?: string[]; limit?: number };
export function useCandidateSearch(p: SearchParams, enabled: boolean) {
  return useQuery({
    queryKey: ek.search(p),
    enabled: enabled && !!p.specialization,
    queryFn: async () =>
      unwrap(
        await api.GET("/employer/candidates/search", {
          params: {
            query: {
              specialization: p.specialization as never,
              grades: p.grades?.length ? p.grades : undefined,
              skills: p.skills?.length ? p.skills : undefined,
              city: p.city || undefined,
              formats: p.formats?.length ? p.formats : undefined,
              has_fsp: (p.has_fsp || undefined) as never,
              grade_statuses: p.grade_statuses?.length ? p.grade_statuses : undefined,
              limit: p.limit ?? 50,
            },
          },
        }),
      ).items ?? [],
  });
}

/** Подписи компонентов ранжирования (docs/03-matching.md). */
export const COMPONENT_LABELS: Record<string, string> = {
  s_test: "тест",
  s_skills: "навыки",
  s_fsp: "ФСП",
  s_fresh: "свежесть",
  s_fit: "условия",
  p_accept: "отклик",
};
export const FACT_KIND_LABELS: Record<string, string> = { test: "Тест", skill: "Навыки", fsp: "ФСП", fresh: "Активность", fit: "Условия", accept: "Отклик" };

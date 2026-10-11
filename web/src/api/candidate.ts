// Данные кандидата: запросы и мутации поверх типизированного клиента. Ключи кешей — в qk.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, isApiError, type Schemas } from "./client";

export type Profile = Schemas["Profile"];
export type Contacts = Schemas["Contacts"];
export type Privacy = Schemas["Privacy"];
export type Survey = Schemas["Survey"];
export type Category = Schemas["Category"];
export type HistoryEntry = Schemas["HistoryEntry"];
export type SessionView = Schemas["SessionView"];
export type ItemView = Schemas["ItemView"];
export type ItemReview = Schemas["ItemReview"];
export type Submission = Schemas["SubmissionStruct"];
export type Invitation = Schemas["Invitation"];
export type Application = Schemas["Application"];
export type Achievement = Schemas["Achievement"];
export type Refdata = Schemas["RefdataOutputBody"];
export type SkillItem = Schemas["SkillItem"];
export type ResumeUpload = Schemas["ResumeUpload"];
export type FSPLink = Schemas["FSPLinkInfo"];

export const qk = {
  profile: ["candidate", "profile"] as const,
  contacts: ["candidate", "contacts"] as const,
  privacy: ["candidate", "privacy"] as const,
  survey: ["candidate", "survey"] as const,
  categories: ["candidate", "categories"] as const,
  sessions: ["candidate", "tests"] as const,
  session: (id: string) => ["candidate", "tests", id] as const,
  invitations: ["candidate", "invitations"] as const,
  invitation: (id: string) => ["candidate", "invitations", id] as const,
  applications: ["candidate", "applications"] as const,
  achievements: ["candidate", "fsp", "achievements"] as const,
  fspLink: ["auth", "fsp", "link"] as const,
  refdata: ["refdata"] as const,
  skills: (q: string, spec: string) => ["skills", q, spec] as const,
};

/** 404 → null вместо ошибки (опрос, привязка ФСП и т. п. могут отсутствовать). */
async function orNull<T>(p: Promise<T>): Promise<T | null> {
  try {
    return await p;
  } catch (e) {
    if (isApiError(e) && e.status === 404) return null;
    throw e;
  }
}

export function useProfile() {
  return useQuery({ queryKey: qk.profile, queryFn: async () => unwrap(await api.GET("/candidate/profile")) });
}
export function useContacts() {
  return useQuery({ queryKey: qk.contacts, queryFn: async () => unwrap(await api.GET("/candidate/contacts")) });
}
export function usePrivacy() {
  return useQuery({ queryKey: qk.privacy, queryFn: async () => unwrap(await api.GET("/candidate/privacy")) });
}
export function useSurvey(specialization: string) {
  return useQuery({
    queryKey: [...qk.survey, specialization],
    enabled: !!specialization,
    queryFn: () => orNull((async () => unwrap(await api.GET("/candidate/survey", { params: { query: { specialization: specialization as never } } })))()),
  });
}
export function useCategories() {
  return useQuery({ queryKey: qk.categories, queryFn: async () => unwrap(await api.GET("/candidate/categories")) });
}
export function useSessions() {
  return useQuery({ queryKey: qk.sessions, queryFn: async () => unwrap(await api.GET("/candidate/tests")).items ?? [] });
}
export function useInvitations() {
  return useQuery({ queryKey: qk.invitations, queryFn: async () => unwrap(await api.GET("/candidate/invitations")).items ?? [] });
}
export function useApplications() {
  return useQuery({ queryKey: qk.applications, queryFn: async () => unwrap(await api.GET("/candidate/applications")).items ?? [] });
}
export function useAchievements(enabled = true) {
  return useQuery({ queryKey: qk.achievements, enabled, queryFn: () => orNull((async () => unwrap(await api.GET("/candidate/fsp/achievements")))()) });
}
export function useFspLink() {
  return useQuery({ queryKey: qk.fspLink, queryFn: async () => unwrap(await api.GET("/auth/fsp/link")) });
}
export function useRefdata() {
  return useQuery({ queryKey: qk.refdata, staleTime: Infinity, queryFn: async () => unwrap(await api.GET("/refdata")) });
}
export function useSkillSearch(q: string, spec: string) {
  return useQuery({
    queryKey: qk.skills(q, spec),
    enabled: q.trim().length >= 2,
    queryFn: async () => unwrap(await api.GET("/skills", { params: { query: { q: q.trim(), spec: (spec || undefined) as never, limit: 20 } } })).items ?? [],
  });
}

/** Инвалидирует всё, что зависит от профиля (обзор, заполненность, категории). */
export function useInvalidateCandidate() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: ["candidate"] });
}

export function useGiveConsent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (kind: "processing" | "publication" | "fsp_data") => unwrap(await api.POST("/auth/consents", { body: { kind } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["auth"] }),
  });
}

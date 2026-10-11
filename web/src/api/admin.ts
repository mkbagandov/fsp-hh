// Данные администратора: сводка, пользователи, банк заданий, сессии с флагами, работодатели, вакансии, аудит.
import { useQuery } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

export type Overview = Schemas["Overview"];
export type AdminUser = Schemas["User"];
export type FamilyStat = Schemas["FamilyStat"];
export type FlaggedSession = Schemas["FlaggedSession"];
export type Employer = Schemas["Employer"];
export type VacancyRow = Schemas["VacancyRow"];
export type AuditEntry = Schemas["AuditEntry"];

export const ak = {
  overview: (days: number) => ["admin", "overview", days] as const,
  users: (p: unknown) => ["admin", "users", p] as const,
  items: ["admin", "items"] as const,
  sessions: ["admin", "sessions"] as const,
  employers: ["admin", "employers"] as const,
  vacancies: ["admin", "vacancies"] as const,
  audit: (p: unknown) => ["admin", "audit", p] as const,
};

export function useOverview(days = 14) {
  return useQuery({ queryKey: ak.overview(days), queryFn: async () => unwrap(await api.GET("/admin/overview", { params: { query: { days } } })) });
}
export function useAdminUsers(p: { q: string; role: string; status: string; offset: number }) {
  return useQuery({
    queryKey: ak.users(p),
    queryFn: async () => unwrap(await api.GET("/admin/users", { params: { query: { q: p.q || undefined, role: (p.role || undefined) as never, status: (p.status || undefined) as never, limit: 50, offset: p.offset } } })),
  });
}
export function useItemStats() {
  return useQuery({ queryKey: ak.items, queryFn: async () => unwrap(await api.GET("/admin/items")).items ?? [] });
}
export function useFlaggedSessions() {
  return useQuery({ queryKey: ak.sessions, queryFn: async () => unwrap(await api.GET("/admin/sessions/flagged", { params: { query: { limit: 200 } } })).items ?? [] });
}
export function useEmployers() {
  return useQuery({ queryKey: ak.employers, queryFn: async () => unwrap(await api.GET("/admin/employers")).items ?? [] });
}
export function useAdminVacancies() {
  return useQuery({ queryKey: ak.vacancies, queryFn: async () => unwrap(await api.GET("/admin/vacancies")).items ?? [] });
}
export function useAudit(p: { action: string; actor: string; offset: number }) {
  return useQuery({
    queryKey: ak.audit(p),
    queryFn: async () => unwrap(await api.GET("/admin/audit", { params: { query: { action: p.action || undefined, actor: p.actor || undefined, limit: 50, offset: p.offset } } })),
  });
}

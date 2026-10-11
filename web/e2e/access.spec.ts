import { expect, test } from "@playwright/test";
import { Api, readyCandidate, readyEmployer, uiLogin } from "./helpers";

// Матрица видимости (ТЗ 2.2): контакты — только после согласия кандидата; чужое — не видно; роли разделены.
test.describe("доступ и приватность", () => {
  test("аноним из кабинета отправляется на вход с возвратом", async ({ page }) => {
    await page.goto("/candidate/profile");
    await expect(page).toHaveURL(/\/login\?next=%2Fcandidate%2Fprofile/);
    await page.goto("/employer/needs");
    await expect(page).toHaveURL(/\/login\?next=%2Femployer%2Fneeds/);
  });

  test("кандидат не попадает в кабинет работодателя, работодатель — в админку", async ({ page, request }) => {
    const cand = await readyCandidate(request, "acc-ui", { withTest: false });
    await uiLogin(page, cand.email);
    await page.goto("/employer");
    await expect(page).toHaveURL(/\/candidate$/);
    await page.goto("/admin");
    await expect(page).toHaveURL(/\/candidate$/);
  });

  test("API: контакты без гранта закрыты, чужие приглашения не видны, роли проверяются", async ({ request }) => {
    const anon = new Api(request);
    expect((await anon.call("GET", "/candidate/profile")).status).toBe(401);
    expect((await anon.call("GET", "/admin/overview")).status).toBe(401);

    const a = await readyCandidate(request, "acc-a", { withTest: false });
    const b = await readyCandidate(request, "acc-b", { withTest: false });
    const emp = await readyEmployer(request, "acc-emp");
    const other = await readyEmployer(request, "acc-other");

    // Роли.
    expect((await a.api.call("GET", "/employer/needs")).status).toBe(403);
    expect((await emp.api.call("GET", "/candidate/profile")).status).toBe(403);
    expect((await emp.api.call("GET", "/admin/users")).status).toBe(403);

    // Карточка без контактов; сами контакты — 403 до принятия приглашения.
    const card = await emp.api.call<{ contacts_granted: boolean; contacts?: unknown }>("GET", `/employer/candidates/${a.id}`, undefined, 200);
    expect(card.body.contacts_granted).toBe(false);
    expect(card.body.contacts).toBeUndefined();
    const denied = await emp.api.call<{ errors?: { value: string }[] }>("GET", `/employer/candidates/${a.id}/contacts`);
    expect(denied.status).toBe(403);
    expect(denied.body.errors?.[0]?.value).toBe("contacts_not_granted");

    // Приглашение: вилка обязательна и валидируется.
    const bad = await emp.api.call("POST", "/employer/invitations", { candidate_id: a.id, message: "Привет", salary_from: 300000, salary_to: 200000, contact_method: { kind: "telegram", value: "@hr" } });
    expect(bad.status).toBe(422);
    const inv = await emp.api.call<{ id: string }>("POST", "/employer/invitations", { candidate_id: a.id, message: "Приглашаем", salary_from: 200000, salary_to: 300000, contact_method: { kind: "telegram", value: "@hr" } }, 201);

    // Чужой кандидат не видит и не может принять приглашение.
    expect((await b.api.call("GET", `/candidate/invitations/${inv.body.id}`)).status).toBe(404);
    expect((await b.api.call("POST", `/candidate/invitations/${inv.body.id}/accept`)).status).toBe(404);
    // Кандидат A видит вилку, но не способ связи до принятия.
    const mine = await a.api.call<{ items: { id: string; contact_method?: unknown; salary_from: number }[] }>("GET", "/candidate/invitations", undefined, 200);
    expect(mine.body.items[0].salary_from).toBe(200000);
    expect(mine.body.items[0].contact_method).toBeUndefined();

    // После принятия контакты видит только пригласившая компания.
    await a.api.call("POST", `/candidate/invitations/${inv.body.id}/accept`, undefined, 200);
    const granted = await emp.api.call<{ telegram?: string }>("GET", `/employer/candidates/${a.id}/contacts`, undefined, 200);
    expect(granted.body.telegram).toBe(a.telegram);
    expect((await other.api.call("GET", `/employer/candidates/${a.id}/contacts`)).status).toBe(403);
  });

  test("скрытый кандидат не виден в поиске, но виден компании, на вакансию которой откликнулся", async ({ request }) => {
    const hidden = await readyCandidate(request, "acc-hidden", { withTest: false });
    await hidden.api.call("PUT", "/candidate/profile", { first_name: hidden.firstName, last_name: hidden.lastName, city: hidden.city, visibility: "hidden" }, 200);
    const emp = await readyEmployer(request, "acc-hid-emp");
    expect((await emp.api.call("GET", `/employer/candidates/${hidden.id}`)).status).toBe(404);
    const vac = await emp.api.call<{ id: string }>("POST", "/employer/vacancies", {
      title: "Скрытый отклик Е2Е",
      description: "Проверка правила: отклик открывает контакты только этой компании.",
      specialization: "backend",
      grade_min: "junior",
      grade_max: "senior",
      salary_from: 150000,
      salary_to: 250000,
    }, 201);
    await emp.api.call("POST", `/employer/vacancies/${vac.body.id}/status`, { status: "published" }, 200);
    await hidden.api.call("POST", `/vacancies/${vac.body.id}/apply`, { cover_letter: "" }, 201);
    expect((await emp.api.call("GET", `/employer/candidates/${hidden.id}`)).status).toBe(200);
    const other = await readyEmployer(request, "acc-hid-other");
    expect((await other.api.call("GET", `/employer/candidates/${hidden.id}`)).status).toBe(404);
  });
});

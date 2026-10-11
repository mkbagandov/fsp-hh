import { expect, test } from "@playwright/test";
import { Api, DEMO_PASSWORD, collectConsoleErrors } from "./helpers";

test.describe("публичная часть", () => {
  test("лендинг объясняет механику и ведёт к регистрации", async ({ page }) => {
    const errors = collectConsoleErrors(page);
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1 })).toContainText("Не вакансии ищут людей");
    await expect(page.getByText("Две стороны, один механизм")).toBeVisible();
    await expect(page.getByText("Вилка обязательна").first()).toBeVisible();
    await page.getByRole("link", { name: /Я работодатель/ }).click();
    await expect(page).toHaveURL(/\/register\?role=employer/);
    await expect(page.getByRole("radio", { name: "Работодатель" })).toHaveAttribute("aria-checked", "true");
    expect(errors, errors.join("\n")).toHaveLength(0);
  });

  test("публичный список вакансий: вилка видна всегда, фильтр по специализации", async ({ page }) => {
    await page.goto("/vacancies");
    await expect(page.getByRole("heading", { name: "Вакансии", level: 1 })).toBeVisible();
    const cards = page.locator("a[href^='/vacancies/']");
    await expect(cards.first()).toBeVisible();
    await expect(cards.first()).toContainText("₽");
    await page.getByLabel("Специализация").selectOption({ label: "Backend" });
    await expect(page).toHaveURL(/specialization=backend/);
    await cards.first().click();
    await expect(page.getByText("вилка, ₽ в месяц")).toBeVisible();
    await expect(page.getByRole("link", { name: "Войти, чтобы откликнуться" })).toBeVisible();
  });

  test("неизвестный адрес — страница 404 в фирменном стиле", async ({ page }) => {
    await page.goto("/нет-такой-страницы");
    await expect(page.getByText("Такой страницы нет")).toBeVisible();
  });

  test("подтверждение категории по QR из PDF-профиля: без персональных данных, чужой id — не найдено", async ({ page, request }) => {
    const api = new Api(request);
    await api.login("demo-candidate@seed.fsp.local", DEMO_PASSWORD);
    const me = await api.call<{ user: { id: string } }>("GET", "/auth/me", undefined, 200);
    const errors = collectConsoleErrors(page);
    await page.goto(`/verify/${me.body.user.id}`);
    await expect(page.getByRole("heading", { name: "Подтверждение категории", level: 1 })).toBeVisible();
    await expect(page.getByText("Подтверждено платформой «ФСП»")).toBeVisible();
    await expect(page.getByText("Backend × Middle")).toBeVisible();
    await expect(page.getByText("подтверждён", { exact: true })).toBeVisible();
    await expect(page.getByText(/Соколов|Артём|@artem|\+7/)).toHaveCount(0);
    await page.goto("/verify/00000000-0000-4000-8000-000000000000");
    await expect(page.getByText("Подтверждение не найдено")).toBeVisible();
    expect(errors, errors.join("\n")).toHaveLength(0);
  });
});

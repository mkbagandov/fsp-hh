import { expect, test } from "@playwright/test";
import { DEMO_PASSWORD, uiLogin } from "./helpers";

test("администратор: сводка, банк заданий, аудит; метрики Prometheus", async ({ page, request }) => {
  await uiLogin(page, "demo-admin@seed.fsp.local", DEMO_PASSWORD);
  await expect(page).toHaveURL(/\/admin$/);
  await expect(page.getByRole("heading", { name: "Сводка платформы" })).toBeVisible();
  await expect(page.getByText("Воронка кандидата")).toBeVisible();
  await page.getByRole("link", { name: "Банк заданий" }).click();
  await expect(page.getByRole("heading", { name: "Банк заданий" })).toBeVisible();
  await expect(page.locator("tbody tr")).not.toHaveCount(0);
  await page.getByRole("link", { name: "Аудит" }).click();
  await expect(page.locator("tbody tr").first()).toBeVisible();

  const metrics = await request.get("/metrics");
  expect(metrics.ok()).toBeTruthy();
  expect(await metrics.text()).toContain("fsp_users{");
});

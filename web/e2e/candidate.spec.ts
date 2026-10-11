import { expect, test } from "@playwright/test";
import { Api, PASSWORD, codeFor, collectConsoleErrors, finishTest, uniqueEmail } from "./helpers";

test("кандидат: регистрация → код → онбординг → адаптивный тест → результат с разбором", async ({ page, request }) => {
  const errors = collectConsoleErrors(page);
  const email = uniqueEmail("cand");

  await test.step("регистрация и подтверждение e-mail", async () => {
    await page.goto("/register?role=candidate");
    await page.getByLabel("E-mail", { exact: true }).fill(email);
    await page.getByLabel("Пароль", { exact: true }).fill(PASSWORD);
    await page.getByRole("checkbox", { name: /Согласен на обработку персональных данных/ }).check();
    await page.getByRole("button", { name: "Создать аккаунт кандидата" }).click();
    await expect(page).toHaveURL(/\/verify\?email=/);
    await page.getByLabel("Код", { exact: true }).fill(await codeFor(request, email));
    await page.getByRole("button", { name: "Подтвердить" }).click();
    await expect(page).toHaveURL(/\/candidate$/);
  });

  await test.step("онбординг: анкета, согласие на публикацию, опрос", async () => {
    await page.getByRole("link", { name: "Начать" }).click();
    await expect(page).toHaveURL(/\/candidate\/onboarding/);
    await page.getByLabel("Имя", { exact: false }).first().fill("Олег");
    await page.getByLabel("Фамилия").fill("Проверкин");
    await page.getByLabel("Город").fill("Томск");
    await page.getByLabel("Основная специализация").selectOption({ label: "Backend-разработка" });
    await page.getByLabel("Ожидание по зарплате, ₽").fill("240000");
    await page.getByRole("button", { name: "Дальше" }).click();
    await page.getByRole("checkbox", { name: /Даю согласие на показ профиля/ }).check();
    await page.getByRole("button", { name: "Дальше" }).click();
    await expect(page.getByRole("radiogroup", { name: "Грейд" })).toBeVisible();
    await page.getByRole("radio", { name: "Middle" }).click();
    await page.getByRole("button", { name: "Дальше" }).click();
    await expect(page.getByRole("heading", { name: /Тест: Backend × Middle/ })).toBeVisible();
    await page.getByRole("button", { name: "Начать тест" }).click();
    await expect(page).toHaveURL(/\/candidate\/tests\//);
  });

  const sessionId = page.url().split("/").pop()!;

  await test.step("первое задание отвечается в интерфейсе", async () => {
    await expect(page.getByText("#1", { exact: true })).toBeVisible();
    const radios = page.locator("input[type=radio][name=answer]");
    const checks = page.locator("main input[type=checkbox]");
    const numeric = page.locator("input[inputmode=decimal]");
    const text = page.locator("main textarea");
    if (await radios.count()) await radios.first().check();
    else if (await checks.count()) await checks.first().check();
    else if (await numeric.count()) await numeric.fill("1");
    else if (await text.count()) await text.first().fill("1");
    const answer = page.getByRole("button", { name: "Ответить" });
    await expect(answer).toBeEnabled();
    await answer.click();
    await expect(page.getByText("#2", { exact: true })).toBeVisible();
  });

  await test.step("остальные задания — через API, итог и разбор в интерфейсе", async () => {
    const api = new Api(request);
    await api.login(email);
    await finishTest(api, sessionId);
    await page.reload();
    await expect(page.getByText("результат теста")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Backend × Middle" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Разбор заданий" })).toBeVisible();
    await expect(page.getByText(/верно|неверно/).first()).toBeVisible();
  });

  await test.step("обзор показывает категорию", async () => {
    await page.getByRole("link", { name: "В кабинет" }).click();
    await expect(page.getByText("основная категория")).toBeVisible();
    await expect(page.getByText("Backend × Middle").first()).toBeVisible();
  });

  expect(errors, errors.join("\n")).toHaveLength(0);
});

test("демо-кандидат видит подтверждённую категорию, достижения ФСП и приглашения", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("E-mail", { exact: true }).fill("demo-candidate@seed.fsp.local");
  await page.getByLabel("Пароль", { exact: true }).fill("Demo1234!");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL(/\/candidate$/);
  await expect(page.getByRole("heading", { name: /Здравствуйте, Артём/ })).toBeVisible();
  await expect(page.getByText("Backend × Middle").first()).toBeVisible();
  await expect(page.getByText("подтверждён", { exact: true }).first()).toBeVisible();
  await page.getByRole("link", { name: "Приглашения" }).first().click();
  await expect(page.getByRole("heading", { name: "Приглашения", level: 1 })).toBeVisible();
});

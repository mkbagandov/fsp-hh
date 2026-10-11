import { expect, test } from "@playwright/test";
import { Api, PASSWORD, collectConsoleErrors, readyCandidate, roleContext, uiLogin, uniqueEmail } from "./helpers";

// Главный сценарий ТЗ: работодатель описывает потребность → видит карту категорий → строит объяснимую подборку →
// приглашает с вилкой; кандидат принимает → контакты раскрываются обеим сторонам.
test("работодатель: потребность → карта → подборка → приглашение → кандидат принимает → контакты", async ({ browser, request }, testInfo) => {
  const cand = await readyCandidate(request, "flow");
  const employerEmail = uniqueEmail("emp");
  await new Api(request).signup("employer", employerEmail);

  const employerCtx = await roleContext(browser, testInfo, "employer");
  const page = await employerCtx.newPage();
  const errors = collectConsoleErrors(page);

  await test.step("вход и создание компании при первом входе", async () => {
    await uiLogin(page, employerEmail);
    await expect(page).toHaveURL(/\/employer\/company\?setup=1/);
    await page.getByLabel("Название").fill("ООО «Сквозной тест»");
    await page.getByLabel("Telegram").fill("@hr_e2e_company");
    await page.getByRole("button", { name: "Создать компанию" }).click();
    await expect(page).toHaveURL(/\/employer$/);
    await expect(page.getByRole("heading", { name: "ООО «Сквозной тест»" })).toBeVisible();
  });

  await test.step("потребность текстом → разбор → структура", async () => {
    await page.goto("/employer/needs/new");
    await page.getByPlaceholder(/Ищем senior Go-разработчика/).fill("Ищем Go-разработчика в платёжную команду: PostgreSQL, Kafka, высокая нагрузка. Удалённо, вилка 220 000 – 320 000 рублей.");
    await page.getByRole("button", { name: "Разобрать в структуру" }).click();
    await expect(page.getByText(/разбор · /)).toBeVisible();
    await expect(page.getByRole("button", { name: /^Go ×/ })).toBeVisible();
    await page.getByLabel("Название").fill("Go-разработчик (сквозной тест)");
    await page.getByRole("combobox", { name: /Грейд от/ }).selectOption("middle");
    await page.getByRole("combobox", { name: /^до/ }).selectOption("senior");
    await expect(page.getByRole("textbox", { name: /Вилка от/ })).toHaveValue("220000");
    await page.getByRole("button", { name: "Создать и открыть карту" }).click();
    await expect(page).toHaveURL(/\/employer\/needs\/[0-9a-f-]{36}$/);
  });

  await test.step("карта категорий с покрытием вилкой", async () => {
    await expect(page.getByText("в диапазоне потребности").first()).toBeVisible();
    await expect(page.getByText(/покрытие вилкой до/).first()).toBeVisible();
    await expect(page.getByText(/Релевантность = test × 0.35/)).toBeVisible();
  });

  await test.step("подборка с фильтром по городу кандидата и объяснения", async () => {
    await page.getByRole("tab", { name: "Подборка" }).click();
    await page.getByLabel("Город").fill(cand.city);
    await page.getByRole("button", { name: "Построить подборку" }).click();
    const row = page.getByRole("row", { name: new RegExp(cand.lastName) });
    await expect(row).toBeVisible();
    await expect(page.getByText("версия 1", { exact: true })).toBeVisible();
    await row.getByRole("button", { name: "почему" }).click();
    await expect(page.getByText(/Условия: ожидание 250 000 ₽/)).toBeVisible();
    await expect(page.getByText(/Нет данных ФСП/)).toBeVisible();
  });

  await test.step("приглашение с вилкой и способом связи", async () => {
    await page.getByRole("row", { name: new RegExp(cand.lastName) }).getByRole("button", { name: "Пригласить" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("textbox", { name: /Вилка от/ })).toHaveValue("220000");
    await dialog.getByLabel("Контакт").fill("@hr_e2e");
    await dialog.getByRole("button", { name: "Отправить приглашение" }).click();
    await expect(page.getByText("Приглашение отправлено")).toBeVisible();
    await expect(page.getByRole("row", { name: new RegExp(cand.lastName) }).getByText("приглашён")).toBeVisible();
  });

  await test.step("до ответа кандидата контакты закрыты", async () => {
    await page.getByRole("link", { name: new RegExp(cand.lastName) }).first().click();
    await expect(page).toHaveURL(new RegExp(`/employer/candidates/${cand.id}`));
    await expect(page.getByText(/Откроются, когда кандидат примет ваше приглашение/)).toBeVisible();
    await expect(page.getByText(cand.telegram)).toHaveCount(0);
  });

  await test.step("кандидат принимает приглашение и видит способ связи компании", async () => {
    const candCtx = await roleContext(browser, testInfo, "candidate");
    const cpage = await candCtx.newPage();
    await uiLogin(cpage, cand.email, PASSWORD);
    await cpage.goto("/candidate/invitations");
    await cpage.getByText("ООО «Сквозной тест»").first().click();
    const dialog = cpage.getByRole("dialog");
    await expect(dialog.getByText("220 000 – 320 000 ₽")).toBeVisible();
    await dialog.getByRole("button", { name: "Принять и открыть контакты" }).click();
    await expect(dialog.getByText("telegram: @hr_e2e", { exact: true })).toBeVisible();
    await expect(dialog.getByText(/telegram: @hr_e2e_company/)).toBeVisible();
    await candCtx.close();
  });

  await test.step("работодатель видит контакты и уведомление о принятии", async () => {
    await page.reload();
    await expect(page.getByText(cand.telegram)).toBeVisible();
    await expect(page.getByText("приглашение: принято")).toBeVisible();
    await page.goto("/notifications");
    await expect(page.getByText("Кандидат принял приглашение").first()).toBeVisible();
  });

  await test.step("обратная связь по потребности учитывает принятие", async () => {
    await page.goto("/employer/needs");
    await page.getByRole("link", { name: "Go-разработчик (сквозной тест)" }).click();
    await page.getByRole("tab", { name: "Обратная связь" }).click();
    await expect(page.getByText("100 %")).toBeVisible();
  });

  expect(errors, errors.join("\n")).toHaveLength(0);
  await employerCtx.close();
});

import { expect, test } from "@playwright/test";
import { PASSWORD, readyCandidate, readyEmployer, roleContext, uiLogin } from "./helpers";

// Классический путь: вакансия с вилкой → публикация → отклик кандидата (открывает контакты) → отказ с причиной.
test("вакансия: черновик → публикация → отклик → отказ с причиной видна кандидату", async ({ browser, request }, testInfo) => {
  const emp = await readyEmployer(request, "vac-emp", "Вакансии Е2Е");
  const cand = await readyCandidate(request, "vac-cand", { withTest: false });
  const title = `QA-автоматизатор Е2Е ${Math.random().toString(36).slice(2, 6)}`;

  const ectx = await roleContext(browser, testInfo, "employer");
  const epage = await ectx.newPage();
  await uiLogin(epage, emp.email);

  let vacancyURL = "";
  await test.step("работодатель создаёт и публикует вакансию", async () => {
    await epage.goto("/employer/vacancies/new");
    await epage.getByLabel("Название").fill(title);
    await epage.getByLabel("Описание").fill("Автотесты на Python и Playwright, API-тесты, CI. Команда из пяти человек, удалённо.");
    await epage.getByLabel("Специализация").selectOption("qa");
    await epage.getByRole("combobox", { name: /Грейд от/ }).selectOption("junior");
    await epage.getByRole("combobox", { name: /^до/ }).selectOption("middle");
    await epage.getByRole("textbox", { name: /Вилка от/ }).fill("120000");
    await epage.getByRole("textbox", { name: /^до, ₽/ }).fill("180000");
    await epage.getByRole("button", { name: "Создать черновик" }).click();
    await expect(epage).toHaveURL(/\/employer\/vacancies\/[0-9a-f-]{36}$/);
    await expect(epage.getByText("черновик", { exact: true })).toBeVisible();
    await epage.getByRole("button", { name: "Опубликовать" }).click();
    await expect(epage.getByText("опубликована").first()).toBeVisible();
    vacancyURL = `/vacancies/${epage.url().split("/").pop()}`;
  });

  await test.step("кандидат откликается и видит отклик в списке", async () => {
    const cctx = await roleContext(browser, testInfo, "candidate");
    const cpage = await cctx.newPage();
    await uiLogin(cpage, cand.email, PASSWORD);
    await cpage.goto(vacancyURL);
    await expect(cpage.getByRole("heading", { name: title })).toBeVisible();
    await expect(cpage.getByText("120 000 – 180 000 ₽").first()).toBeVisible();
    await cpage.getByRole("button", { name: "Откликнуться" }).click();
    await cpage.getByPlaceholder("Почему вам интересна эта роль").fill("Пишу автотесты на Playwright три года.");
    await cpage.getByRole("button", { name: "Отправить отклик" }).click();
    await expect(cpage).toHaveURL(/\/candidate\/applications/);
    await expect(cpage.getByRole("link", { name: title })).toBeVisible();
    await expect(cpage.getByText("отправлен", { exact: true })).toBeVisible();
    await cctx.close();
  });

  await test.step("работодатель видит отклик, контакты открыты, отказывает с причиной", async () => {
    await epage.reload();
    await expect(epage.getByText(`${cand.lastName} ${cand.firstName}`)).toBeVisible();
    await expect(epage.getByText("Пишу автотесты на Playwright три года.")).toBeVisible();
    await epage.getByRole("button", { name: "Отказать" }).click();
    const dialog = epage.getByRole("dialog");
    await dialog.getByLabel("Причина").fill("Нужен опыт нагрузочного тестирования");
    await dialog.getByRole("button", { name: "Отказать" }).click();
    await expect(epage.getByText("Отказ: Нужен опыт нагрузочного тестирования")).toBeVisible();
    await epage.goto(`/employer/candidates/${cand.id}`);
    await expect(epage.getByText(cand.telegram)).toBeVisible();
  });

  await test.step("кандидат видит причину отказа", async () => {
    const cctx = await roleContext(browser, testInfo, "candidate");
    const cpage = await cctx.newPage();
    await uiLogin(cpage, cand.email, PASSWORD);
    await cpage.goto("/candidate/applications");
    await expect(cpage.getByText("Причина отказа: Нужен опыт нагрузочного тестирования")).toBeVisible();
    await cctx.close();
  });
  await ectx.close();
});

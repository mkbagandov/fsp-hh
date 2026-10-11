import { test, expect } from "@playwright/test";
import { Api, uniqueEmail, uiLogin, collectConsoleErrors } from "./helpers";

// Сквозной путь загрузки резюме через настоящий бинарник API: текст извлекается в изолированном
// дочернем процессе (pdftext.Isolated), разбирается эмулятором LLM и применяется только после подтверждения.
test("резюме: PDF-профиль платформы → загрузка другим кандидатом → черновик → применение", async ({ page, request }) => {
  const errors = collectConsoleErrors(page);
  const letters = "бвгджзклмнпрст";
  const lastName = "Ре" + Array.from({ length: 4 }, () => letters[Math.floor(Math.random() * letters.length)]).join("") + "ова";

  const src = new Api(request);
  await src.signup("candidate", uniqueEmail("resume-src"));
  await src.call("PUT", "/candidate/profile", {
    first_name: "Алиса",
    last_name: lastName,
    city: "Казань",
    work_formats: ["remote"],
    visibility: "open",
    primary_specialization: "backend",
    headline: "Backend-разработчик на Go",
  }, 200);
  const pdfRes = await request.get("/api/v1/candidate/profile.pdf", { headers: { Authorization: `Bearer ${src.token}` } });
  expect(pdfRes.status()).toBe(200);
  const pdf = await pdfRes.body();
  expect(pdf.subarray(0, 4).toString()).toBe("%PDF");

  // Аноним получает отказ до разбора multipart-тела.
  const anon = await request.post("/api/v1/candidate/resume", { multipart: { file: { name: "cv.pdf", mimeType: "application/pdf", buffer: pdf } } });
  expect(anon.status()).toBe(401);

  const dst = new Api(request);
  const email = uniqueEmail("resume-dst");
  await dst.signup("candidate", email);
  await uiLogin(page, email);
  await page.goto("/candidate/profile?tab=resume");
  await page.locator('input[type="file"]').setInputFiles({ name: "profile.pdf", mimeType: "application/pdf", buffer: pdf });
  await expect(page.getByText("разбор: mock")).toBeVisible();
  await expect(page.getByText(`${lastName} Алиса`)).toBeVisible();
  await page.getByRole("button", { name: "Применить выбранное" }).click();
  await expect(page.getByText("Данные из резюме применены")).toBeVisible();

  const prof = await dst.call<{ first_name: string; last_name: string; resume_uploaded: boolean }>("GET", "/candidate/profile", undefined, 200);
  expect(prof.body).toMatchObject({ first_name: "Алиса", last_name: lastName, resume_uploaded: true });
  expect(errors).toEqual([]);
});

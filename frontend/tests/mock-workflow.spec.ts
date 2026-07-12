import { expect, test } from "@playwright/test";

test("operator can inspect request, import row, and protocol gate", async ({ page }) => {
  await page.goto("/requests");
  await expect(page.getByText("ООО Тест-Сервис")).toBeVisible();

  await page.goto("/imports/import-1");
  await expect(page.getByText("Петров Петр Петрович")).toBeVisible();
  await page.getByRole("button", { name: "Пропустить row-2" }).click();
  const skippedRow = page.getByRole("row", { name: /Петров Петр Петрович/ });
  await expect(skippedRow).toContainText("row-2");
  await expect(skippedRow).toContainText("skipped");

  await page.goto("/protocols/protocol-2605-a-15");
  await expect(page.getByRole("heading", { name: /2605А15/ })).toBeVisible();
  await expect(page.getByText(/Заполните номера Минтруда/)).toBeVisible();
});

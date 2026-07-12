import { expect, test } from "@playwright/test";

test("protocol workflow mock screen is visible", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { name: /2605А15/i })).toBeVisible();
  await expect(page.getByText(/Реестровые номера/i)).toBeVisible();
  await expect(page.getByText(/Заполните номера Минтруда/i)).toBeVisible();
});

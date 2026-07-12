import { expect, test } from "@playwright/test";

const routes = [
  ["/dashboard", "Рабочий стол"],
  ["/requests", "Заявки"],
  ["/imports", "Импорт"],
  ["/protocols", "Протоколы"],
  ["/documents", "Документы"],
  ["/moodle", "Moodle"],
  ["/workers", "Слушатели"],
  ["/employers", "Работодатели"],
  ["/programs", "Программы"],
  ["/audit", "Журнал"],
  ["/analytics", "Аналитика"],
  ["/users", "Пользователи и роли"],
  ["/notifications", "Уведомления"],
  ["/settings", "Настройки"],
] as const;

const stageTwoRoutes = [
  ["/", "Рабочий стол"],
  ["/requests/request-1", "Карточка заявки"],
  ["/workers/worker-1", "Карточка слушателя"],
  ["/employers/employer-1", "Карточка работодателя"],
] as const;

test.describe("app shell routes", () => {
  for (const [path, heading] of routes) {
    test(`renders ${path}`, async ({ page }) => {
      await page.goto(path);
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
    });
  }

  for (const [path, heading] of stageTwoRoutes) {
    test(`renders Stage 2 route ${path}`, async ({ page }) => {
      await page.goto(path);
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
    });
  }
});

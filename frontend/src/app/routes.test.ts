import { describe, expect, it } from "vitest";

import { appRoutes, navGroups, routeIds, routesByPath } from "./routes";

describe("routes", () => {
  it("contains the approved grouped navigation without Mintrud API", () => {
    expect(routeIds).toContain("dashboard");
    expect(navGroups.map((group) => group.label)).toEqual([
      "Операции",
      "Реестр",
      "Контроль",
      "Администрирование",
    ]);
    expect(navGroups.flatMap((group) => group.items.map((item) => item.label))).toEqual([
      "Заявки",
      "Импорт",
      "Протоколы",
      "Документы",
      "Moodle",
      "Слушатели",
      "Работодатели",
      "Программы",
      "Журнал",
      "Аналитика",
      "Пользователи и роли",
      "Уведомления",
      "Настройки",
    ]);
    expect(appRoutes.some((route) => route.label.includes("Минтруд API"))).toBe(false);
  });

  it("keeps route and navigation metadata internally consistent", () => {
    expect(new Set(routeIds).size).toBe(appRoutes.length);
    expect(new Set(appRoutes.map((route) => route.path)).size).toBe(appRoutes.length);
    expect(routesByPath["/missing-route"]).toBeUndefined();
    expect(navGroups.flatMap((group) => group.items).every((route) => !route.path.includes(":"))).toBe(true);
  });
});

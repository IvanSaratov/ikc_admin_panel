import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { DataTable } from "./DataTable";

interface Row {
  name: string;
  status: string;
}

const rows: Row[] = [
  { name: "ООО Тест-Сервис", status: "review" },
  { name: "АО Пример-Проект", status: "ready" },
];

describe("DataTable", () => {
  afterEach(() => {
    cleanup();
  });

  it("filters rows with the table search", async () => {
    const user = userEvent.setup();

    render(
      <DataTable
        ariaLabel="Тестовая таблица"
        data={rows}
        columns={[
          { accessorKey: "name", header: "Название" },
          { accessorKey: "status", header: "Статус" },
        ]}
      />
    );

    expect(screen.getByRole("table", { name: "Тестовая таблица" })).toBeInTheDocument();

    await user.type(screen.getByRole("searchbox", { name: "Фильтр таблицы" }), "Пример");

    expect(screen.queryByText("ООО Тест-Сервис")).not.toBeInTheDocument();
    expect(screen.getByText("АО Пример-Проект")).toBeInTheDocument();
  });

  it("shows a no-results row when filtering removes every row", async () => {
    const user = userEvent.setup();

    render(
      <DataTable
        ariaLabel="Тестовая таблица"
        data={rows}
        columns={[
          { accessorKey: "name", header: "Название" },
          { accessorKey: "status", header: "Статус" },
        ]}
      />
    );

    await user.type(screen.getByRole("searchbox", { name: "Фильтр таблицы" }), "Не найдено");

    expect(screen.getByText("Нет строк для отображения")).toBeInTheDocument();
  });

  it("only renders sortable headers as buttons and exposes sort state", async () => {
    const user = userEvent.setup();

    render(
      <DataTable
        ariaLabel="Тестовая таблица"
        data={rows}
        columns={[
          { accessorKey: "name", header: "Название" },
          { id: "actions", header: "Действия", enableSorting: false, cell: () => <button type="button">Открыть</button> },
        ]}
      />
    );

    expect(screen.getByRole("columnheader", { name: "Действия" })).not.toContainElement(
      screen.queryByRole("button", { name: "Действия" })
    );

    const nameHeader = screen.getByRole("columnheader", { name: "Название" });
    await user.click(screen.getByRole("button", { name: "Название" }));

    expect(nameHeader).toHaveAttribute("aria-sort", "ascending");
  });
});

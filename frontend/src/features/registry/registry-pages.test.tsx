import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it } from "vitest";

import { EmployersPage } from "../employers/EmployersPage";
import { ProgramsPage } from "../programs/ProgramsPage";
import { WorkersPage } from "../workers/WorkersPage";

describe("registry pages", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders workers registry", async () => {
    render(<WorkersPage />, { wrapper: MemoryRouter });
    expect(await screen.findByRole("heading", { name: "Слушатели" })).toBeInTheDocument();
    expect(screen.getByText("Иванов Иван Иванович")).toBeInTheDocument();
  });

  it("renders employers registry", async () => {
    render(<EmployersPage />, { wrapper: MemoryRouter });
    expect(await screen.findByRole("heading", { name: "Работодатели" })).toBeInTheDocument();
    expect(screen.getByText("ООО Тест-Сервис")).toBeInTheDocument();
  });

  it("renders programs registry", async () => {
    render(<ProgramsPage />, { wrapper: MemoryRouter });
    expect(await screen.findByRole("heading", { name: "Программы" })).toBeInTheDocument();
    expect(screen.getByText("Общие вопросы охраны труда")).toBeInTheDocument();
  });
});

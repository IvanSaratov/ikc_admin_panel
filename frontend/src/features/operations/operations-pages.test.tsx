import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { resetMockStore } from "../../api/mock/services";
import { ImportDetailPage } from "../imports/ImportDetailPage";
import { MoodlePage } from "../moodle/MoodlePage";
import { ProtocolDetailPage } from "../protocols/ProtocolDetailPage";
import { RequestDetailPage } from "../requests/RequestDetailPage";
import { RequestsPage } from "../requests/RequestsPage";

describe("operations pages", () => {
  beforeEach(() => {
    resetMockStore();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders requests queue", async () => {
    render(<RequestsPage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: "Заявки" })).toBeInTheDocument();
    expect(screen.getByText("ООО Тест-Сервис")).toBeInTheDocument();
  });

  it("resolves import rows", async () => {
    const user = userEvent.setup();

    render(<ImportDetailPage />, { wrapper: MemoryRouter });

    expect(await screen.findByText("Петров Петр Петрович")).toBeInTheDocument();
    expect(screen.getByText("conflict")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Пропустить row-2" }));
    expect(await screen.findByText("skipped")).toBeInTheDocument();
  });

  it("renders protocol gate reason", async () => {
    render(<ProtocolDetailPage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: /2605А15/ })).toBeInTheDocument();
    expect(screen.getByText(/Заполните номера Минтруда/)).toBeInTheDocument();
  });

  it("renders moodle queue", async () => {
    render(<MoodlePage />, { wrapper: MemoryRouter });

    expect(await screen.findByRole("heading", { name: "Moodle" })).toBeInTheDocument();
    expect(screen.getByText("review_required")).toBeInTheDocument();
  });

  it("uses request route param for detail lookup", async () => {
    render(
      <MemoryRouter initialEntries={["/requests/request-missing"]}>
        <Routes>
          <Route path="/requests/:requestId" element={<RequestDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "Заявка не найдена" })).toBeInTheDocument();
  });

  it("uses import route param for row lookup", async () => {
    render(
      <MemoryRouter initialEntries={["/imports/import-missing"]}>
        <Routes>
          <Route path="/imports/:importId" element={<ImportDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "Разбор импорта" })).toBeInTheDocument();
    expect(screen.queryByText("Петров Петр Петрович")).not.toBeInTheDocument();
  });

  it("uses protocol route param for detail lookup", async () => {
    render(
      <MemoryRouter initialEntries={["/protocols/protocol-missing"]}>
        <Routes>
          <Route path="/protocols/:protocolId" element={<ProtocolDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "Не удалось загрузить протокол" })).toBeInTheDocument();
  });
});

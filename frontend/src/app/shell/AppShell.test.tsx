import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it } from "vitest";

import { AppShell } from "./AppShell";

function renderShell(path = "/dashboard") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/dashboard" element={<h1>Рабочий стол</h1>} />
          <Route path="/requests" element={<h1>Заявки</h1>} />
          <Route path="/requests/:requestId" element={<h1>Карточка заявки</h1>} />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

describe("AppShell", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders the admin shell around routed content", () => {
    renderShell();

    expect(screen.getByRole("heading", { name: "Рабочий стол" })).toBeInTheDocument();
    expect(screen.getByText("ИКЦ Эксперт")).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "Поиск по админке" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Рабочий стол" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Операции/ })).toBeInTheDocument();
  });

  it("opens an operations group and navigates to requests", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(screen.getByRole("button", { name: /Операции/ }));
    await user.click(screen.getByRole("link", { name: "Заявки" }));

    expect(screen.getByRole("heading", { name: "Заявки" })).toBeInTheDocument();
  });

  it("opens the parent navigation group for detail routes", () => {
    renderShell("/requests/123");

    expect(screen.getByRole("button", { name: /Операции/ })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("link", { name: "Заявки" })).toHaveClass("is-active");
    expect(screen.getByRole("heading", { name: "Карточка заявки" })).toBeInTheDocument();
  });
});

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AuditPage } from "../audit/AuditPage";
import { SettingsPage } from "../settings/SettingsPage";

describe("control pages", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders audit events", async () => {
    render(<AuditPage />);

    expect(await screen.findByRole("heading", { name: "Журнал" })).toBeInTheDocument();
    expect(screen.getByText("import.row.conflict")).toBeInTheDocument();
  });

  it("renders settings panels", () => {
    render(<SettingsPage />);

    expect(screen.getByRole("heading", { name: "Настройки" })).toBeInTheDocument();
    expect(screen.getByText("Режим данных")).toBeInTheDocument();
    expect(screen.getByText("Backup SQLite")).toBeInTheDocument();
  });
});

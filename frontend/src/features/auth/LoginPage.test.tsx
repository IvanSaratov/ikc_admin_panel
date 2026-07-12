import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  afterEach(() => {
    cleanup();
  });

  it("submits login credentials", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn().mockResolvedValue(undefined);

    render(<LoginPage onLogin={onLogin} />);

    await user.type(screen.getByLabelText("Логин"), "alice");
    await user.type(screen.getByLabelText("Пароль"), "test-password");
    await user.click(screen.getByRole("button", { name: "Войти" }));

    expect(onLogin).toHaveBeenCalledWith({ login: "alice", password: "test-password" });
  });

  it("shows an alert when login rejects", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn().mockRejectedValue(new Error("invalid credentials"));

    render(<LoginPage onLogin={onLogin} />);

    await user.type(screen.getByLabelText("Логин"), "alice");
    await user.type(screen.getByLabelText("Пароль"), "WRONG");
    await user.click(screen.getByRole("button", { name: "Войти" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось войти");
  });
});

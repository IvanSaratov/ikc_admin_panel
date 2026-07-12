import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

describe("App motion/accessibility shell", () => {
  afterEach(() => {
    window.history.pushState({}, "", "/");
    vi.resetModules();
  });

  it("renders through MotionConfig and keeps login route accessible", async () => {
    window.history.pushState({}, "", "/login");
    const { App } = await import("./App");

    render(<App />);

    expect(await screen.findByRole("heading", { name: /Вход/ })).toBeInTheDocument();
  });
});

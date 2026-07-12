import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DetailDrawer } from "./DetailDrawer";

describe("DetailDrawer", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders as a modal dialog and closes from the close button", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();

    render(
      <DetailDrawer title="Карточка строки" open onClose={onClose}>
        <p>Детали импорта</p>
      </DetailDrawer>
    );

    expect(screen.getByRole("dialog", { name: "Карточка строки" })).toHaveAttribute("aria-modal", "true");
    expect(screen.getByText("Детали импорта")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Закрыть" }));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes on Escape and does not render when closed", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const { rerender } = render(
      <DetailDrawer title="Карточка строки" open onClose={onClose}>
        <p>Детали импорта</p>
      </DetailDrawer>
    );

    await user.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalledTimes(1);

    rerender(
      <DetailDrawer title="Карточка строки" open={false} onClose={onClose}>
        <p>Детали импорта</p>
      </DetailDrawer>
    );

    expect(screen.queryByRole("dialog", { name: "Карточка строки" })).not.toBeInTheDocument();
  });

  it("keeps tab focus inside the modal drawer", async () => {
    const user = userEvent.setup();

    render(
      <div>
        <button type="button">Фоновое действие</button>
        <DetailDrawer title="Карточка строки" open onClose={vi.fn()}>
          <button type="button">Внутреннее действие</button>
        </DetailDrawer>
      </div>
    );

    const closeButton = screen.getByRole("button", { name: "Закрыть" });
    const innerButton = screen.getByRole("button", { name: "Внутреннее действие" });

    expect(closeButton).toHaveFocus();

    await user.tab();
    expect(innerButton).toHaveFocus();

    await user.tab();
    expect(closeButton).toHaveFocus();

    await user.tab({ shift: true });
    expect(innerButton).toHaveFocus();
  });
});

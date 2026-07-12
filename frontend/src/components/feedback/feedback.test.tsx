import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { EmptyState } from "./EmptyState";
import { ErrorState } from "./ErrorState";
import { InDevelopmentPage } from "./InDevelopmentPage";
import { LoadingState } from "./LoadingState";

describe("feedback components", () => {
  it("renders empty state with action", () => {
    const onAction = vi.fn();
    const { container, rerender } = render(
      <EmptyState
        title="Нет заявок"
        description="Загрузите XLSX"
        actionLabel="Новая заявка"
        onAction={onAction}
      />
    );

    expect(container.querySelector(".empty-state")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Нет заявок" })).toBeInTheDocument();
    expect(screen.getByText("Загрузите XLSX")).toBeInTheDocument();
    const action = screen.getByRole("button", { name: "Новая заявка" });
    expect(action).toHaveClass("button", "button-primary");
    expect(action).toHaveAttribute("type", "button");

    rerender(<EmptyState title="Нет заявок" description="Загрузите XLSX" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("renders loading status accessibly", () => {
    const { container } = render(<LoadingState label="Загрузка заявок" />);

    expect(screen.getByRole("status", { name: "Загрузка заявок" })).toBeInTheDocument();
    expect(screen.getByText("Загрузка заявок")).toBeInTheDocument();
    expect(container.querySelector(".loading-dot")).toHaveAttribute("aria-hidden", "true");
  });

  it("renders error state with retry", () => {
    const onAction = vi.fn();
    const { container, rerender } = render(
      <ErrorState
        title="Ошибка импорта"
        description="Файл не прочитан"
        actionLabel="Повторить"
        onAction={onAction}
      />
    );

    const alert = screen.getByRole("alert");
    expect(alert).toHaveClass("error-state");
    expect(alert).toHaveTextContent("Ошибка импорта");
    expect(alert).toHaveTextContent("Файл не прочитан");
    const action = screen.getByRole("button", { name: "Повторить" });
    expect(action).toHaveClass("button", "button-secondary");
    expect(action).toHaveAttribute("type", "button");
    expect(container.querySelector(".error-state")).toBeInTheDocument();

    rerender(<ErrorState title="Ошибка импорта" description="Файл не прочитан" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("renders in-development page with planned capabilities", () => {
    render(
      <InDevelopmentPage
        title="Аналитика"
        description="Раздел готовится"
        planned={["Динамика заявок", "Статусы протоколов"]}
      />
    );

    expect(screen.getByRole("heading", { name: "Аналитика" })).toBeInTheDocument();
    expect(screen.getByText("Сейчас в разработке")).toBeInTheDocument();
    expect(screen.getByText("Раздел готовится")).toBeInTheDocument();
    const panel = screen.getByRole("heading", { name: "Планируемые возможности" }).closest(".in-dev-panel");
    expect(panel).not.toBeNull();
    expect(within(panel as HTMLElement).getByText("Динамика заявок")).toBeInTheDocument();
    expect(within(panel as HTMLElement).getByText("Статусы протоколов")).toBeInTheDocument();
  });
});

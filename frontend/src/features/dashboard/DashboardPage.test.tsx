import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ClientRequest } from "../../api/mock/types";

import { DashboardPage } from "./DashboardPage";

const requests: ClientRequest[] = [
  {
    id: "request-1",
    employerId: "employer-1",
    employerName: "ООО Тест-Сервис",
    receivedDate: "2026-07-08",
    status: "review",
    rowsTotal: 18,
    rowsNeedReview: 7,
    nextAction: "Разобрать конфликты импорта",
    attention: "danger",
  },
];

describe("DashboardPage", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders operational metrics and attention queue", async () => {
    render(<DashboardPage loadRequests={() => Promise.resolve(requests)} />);

    expect(await screen.findByRole("heading", { name: "Рабочий стол" })).toBeInTheDocument();
    expect(screen.getByText("Требуют внимания")).toBeInTheDocument();
    expect(screen.getByText(/ООО Тест-Сервис/)).toBeInTheDocument();
    expect(screen.getByText("Mock data")).toBeInTheDocument();
  });

  it("renders a loading state before requests resolve", () => {
    render(<DashboardPage loadRequests={() => new Promise(() => undefined)} />);

    expect(screen.getByRole("status", { name: "Загрузка рабочего стола" })).toBeInTheDocument();
  });

  it("renders an empty attention queue", async () => {
    render(
      <DashboardPage
        loadRequests={() =>
          Promise.resolve([
            {
              ...requests[0],
              attention: "normal",
            },
          ])
        }
      />
    );

    expect(await screen.findByRole("heading", { name: "Очередь пуста" })).toBeInTheDocument();
  });

  it("renders an error state and retries loading", async () => {
    const user = userEvent.setup();
    const loadRequests = vi
      .fn()
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce(requests);

    render(<DashboardPage loadRequests={loadRequests} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось загрузить рабочий стол");
    await user.click(screen.getByRole("button", { name: "Повторить" }));

    expect(await screen.findByText(/ООО Тест-Сервис/)).toBeInTheDocument();
    expect(loadRequests).toHaveBeenCalledTimes(2);
  });
});

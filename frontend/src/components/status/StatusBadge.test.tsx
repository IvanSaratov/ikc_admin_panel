import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { StatusBadge } from "./StatusBadge";

describe("StatusBadge", () => {
  it("renders a danger status badge", () => {
    render(<StatusBadge label="blocked" tone="danger" />);

    expect(screen.getByText("blocked")).toHaveClass("status-badge-danger");
  });

  it("uses the neutral tone by default", () => {
    render(<StatusBadge label="ready" />);

    expect(screen.getByText("ready")).toHaveClass("status-badge-neutral");
  });
});

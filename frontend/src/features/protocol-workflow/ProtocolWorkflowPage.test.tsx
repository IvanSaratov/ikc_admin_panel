import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { mockProtocolWorkflow } from "../../api/mockProtocolWorkflow";
import { ProtocolWorkflowPage } from "./ProtocolWorkflowPage";

describe("ProtocolWorkflowPage", () => {
  it("renders the protocol number and blocked reason", () => {
    render(<ProtocolWorkflowPage workflow={mockProtocolWorkflow} />);

    expect(screen.getByRole("heading", { name: /2605А15/i })).toBeInTheDocument();
    expect(screen.getByText(/Заполните номера Минтруда/i)).toBeInTheDocument();
  });
});

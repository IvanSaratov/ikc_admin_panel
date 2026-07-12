import { beforeEach, describe, expect, it } from "vitest";
import * as client from "../client";
import {
  getProtocolWorkflow,
  listAuditEvents,
  listEmployers,
  listImportRows,
  listPrograms,
  listRequests,
  listWorkers,
  resetMockStore,
  resolveImportRow
} from "./services";

describe("mock services", () => {
  beforeEach(() => {
    resetMockStore();
  });

  it("uses obviously synthetic fixture data", async () => {
    const workers = await listWorkers();
    const employers = await listEmployers();

    expect(workers.every((worker) => worker.email.endsWith(".example") || worker.email.endsWith("example.test"))).toBe(true);
    expect(employers.every((employer) => employer.name.includes("Тест") || employer.name.includes("Пример"))).toBe(true);
  });

  it("connects request, import rows, and protocol workflow", async () => {
    const requests = await listRequests();
    const rows = await listImportRows("import-1");
    const workflow = await getProtocolWorkflow("protocol-2605-a-15");

    expect(requests[0].id).toBe("request-1");
    expect(rows.some((row) => row.status === "conflict")).toBe(true);
    expect(workflow.stages.map((stage) => stage.id)).toEqual([
      "participants",
      "fix",
      "xml",
      "registry",
      "docx",
      "closed"
    ]);
  });

  it("updates import row status in the mock store", async () => {
    await resolveImportRow("row-2", "skipped");
    const rows = await listImportRows("import-1");

    expect(rows.find((row) => row.id === "row-2")?.status).toBe("skipped");
  });

  it("returns cloned snapshots so callers cannot mutate the store", async () => {
    const rows = await listImportRows("import-1");
    rows[0].status = "invalid";

    const freshRows = await listImportRows("import-1");

    expect(freshRows.find((row) => row.id === rows[0].id)?.status).not.toBe("invalid");
  });

  it("exposes mock services through the public client", async () => {
    await expect(client.listRequests()).resolves.toEqual(
      expect.arrayContaining([expect.objectContaining({ id: "request-1" })])
    );
  });

  it("exposes programs and audit events", async () => {
    await expect(listPrograms()).resolves.toHaveLength(5);
    await expect(listAuditEvents()).resolves.toEqual(
      expect.arrayContaining([expect.objectContaining({ action: "import.row.conflict" })])
    );
  });
});

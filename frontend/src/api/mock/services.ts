import { getMockSnapshot, resetMockStore, setImportRowStatus } from "./mockStore";
import type { ImportRowStatus } from "./types";

const wait = () => new Promise((resolve) => globalThis.setTimeout(resolve, 20));

export { resetMockStore };

export async function listRequests() {
  await wait();
  return getMockSnapshot().requests;
}

export async function listImportRuns() {
  await wait();
  return getMockSnapshot().importRuns;
}

export async function listImportRows(importId: string) {
  await wait();
  return getMockSnapshot().importRows.filter((row) => row.importId === importId);
}

export async function resolveImportRow(rowId: string, status: ImportRowStatus) {
  await wait();
  setImportRowStatus(rowId, status);
}

export async function listProtocols() {
  await wait();
  return getMockSnapshot().protocols;
}

export async function getProtocolWorkflow(protocolId: string) {
  await wait();
  const protocol = getMockSnapshot().protocols.find((item) => item.id === protocolId);
  if (!protocol) {
    throw new Error(`Protocol ${protocolId} not found`);
  }
  return protocol;
}

export async function listWorkers() {
  await wait();
  return getMockSnapshot().workers;
}

export async function listEmployers() {
  await wait();
  return getMockSnapshot().employers;
}

export async function listPrograms() {
  await wait();
  return getMockSnapshot().programs;
}

export async function listGenerationRuns() {
  await wait();
  return getMockSnapshot().generationRuns;
}

export async function listMoodleAccounts() {
  await wait();
  return getMockSnapshot().moodleAccounts;
}

export async function listAuditEvents() {
  await wait();
  return getMockSnapshot().auditEvents;
}

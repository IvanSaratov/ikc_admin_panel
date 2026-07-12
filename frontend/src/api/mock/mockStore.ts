import { auditEvents, employers, generationRuns, importRows, importRuns, moodleAccounts, programs, protocols, requests, workers } from "./fixtures";
import type { ImportRowStatus } from "./types";

function createStore() {
  return {
    requests: structuredClone(requests),
    importRuns: structuredClone(importRuns),
    importRows: structuredClone(importRows),
    protocols: structuredClone(protocols),
    workers: structuredClone(workers),
    employers: structuredClone(employers),
    programs: structuredClone(programs),
    generationRuns: structuredClone(generationRuns),
    moodleAccounts: structuredClone(moodleAccounts),
    auditEvents: structuredClone(auditEvents)
  };
}

let store = createStore();

export function resetMockStore() {
  store = createStore();
}

const clone = <T>(value: T): T => structuredClone(value);

export function getMockStore() {
  return store;
}

export function getMockSnapshot() {
  return {
    requests: clone(store.requests),
    importRuns: clone(store.importRuns),
    importRows: clone(store.importRows),
    protocols: clone(store.protocols),
    workers: clone(store.workers),
    employers: clone(store.employers),
    programs: clone(store.programs),
    generationRuns: clone(store.generationRuns),
    moodleAccounts: clone(store.moodleAccounts),
    auditEvents: clone(store.auditEvents)
  };
}

export function setImportRowStatus(rowId: string, status: ImportRowStatus) {
  const row = store.importRows.find((item) => item.id === rowId);
  if (!row) {
    throw new Error(`Import row ${rowId} not found`);
  }
  row.status = status;
  row.reason = status === "skipped" ? "Оператор пропустил строку в mock workflow" : row.reason;
}

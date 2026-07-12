export type LoginInput = {
  login: string;
  password: string;
};

export type LoginResponse = {
  authenticated: boolean;
  login: string;
};

export async function login(input: LoginInput): Promise<LoginResponse> {
  const response = await fetch("/api/login", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(input),
  });

  if (!response.ok) {
    throw new Error("Login failed");
  }

  return response.json() as Promise<LoginResponse>;
}

export {
  getProtocolWorkflow,
  listAuditEvents,
  listEmployers,
  listGenerationRuns,
  listImportRows,
  listImportRuns,
  listMoodleAccounts,
  listPrograms,
  listProtocols,
  listRequests,
  listWorkers,
  resolveImportRow
} from "./mock/services";

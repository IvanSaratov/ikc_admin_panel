import type { ProtocolWorkflow } from "./mockProtocolWorkflow";

export async function getProtocolWorkflow(protocolId: number): Promise<ProtocolWorkflow> {
  const response = await fetch(`/api/protocols/${protocolId}/workflow`, {
    headers: { Accept: "application/json" }
  });
  if (!response.ok) {
    throw new Error(`Failed to load protocol workflow: ${response.status}`);
  }
  return response.json() as Promise<ProtocolWorkflow>;
}

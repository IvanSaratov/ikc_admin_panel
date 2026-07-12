export type WorkflowStageState = "done" | "active" | "blocked" | "pending";

export interface WorkflowStage {
  id: string;
  label: string;
  state: WorkflowStageState;
  reason?: string;
}

export interface ProtocolWorkflow {
  protocolId: number;
  number: string;
  employer: string;
  stages: WorkflowStage[];
}

export const mockProtocolWorkflow: ProtocolWorkflow = {
  protocolId: 1,
  number: "2605А15",
  employer: "Тестовый работодатель",
  stages: [
    { id: "participants", label: "Участники", state: "done" },
    { id: "fix", label: "Фиксация", state: "done" },
    { id: "xml", label: "XML", state: "active" },
    {
      id: "registry",
      label: "Реестровые номера",
      state: "blocked",
      reason: "Заполните номера Минтруда для всех активных участников"
    },
    { id: "docx", label: "DOCX", state: "pending" },
    { id: "closed", label: "Закрытие", state: "pending" }
  ]
};

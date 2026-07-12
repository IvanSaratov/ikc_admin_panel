export type AttentionLevel = "normal" | "warning" | "danger";
export type RequestStatus = "review" | "ready" | "completed" | "cancelled";
export type ImportRowStatus = "new" | "duplicate" | "conflict" | "requires_review" | "invalid" | "imported" | "skipped";
export type ProtocolStageState = "done" | "active" | "blocked" | "pending";
export type GenerationStatus = "success" | "failed" | "needs_rebuild" | "running";
export type MoodleStatus = "not_started" | "queued" | "enrolled" | "review_required" | "failed";

export interface ClientRequest {
  id: string;
  employerId: string;
  employerName: string;
  receivedDate: string;
  status: RequestStatus;
  rowsTotal: number;
  rowsNeedReview: number;
  nextAction: string;
  attention: AttentionLevel;
}

export interface ImportRun {
  id: string;
  requestId: string;
  fileName: string;
  uploadedAt: string;
  rowsTotal: number;
  status: "review" | "completed";
}

export interface ImportRow {
  id: string;
  importId: string;
  rowNumber: number;
  fullName: string;
  snils: string;
  email: string;
  employerName: string;
  position: string;
  programs: string[];
  status: ImportRowStatus;
  reason: string;
}

export interface ProtocolStage {
  id: "participants" | "fix" | "xml" | "registry" | "docx" | "closed";
  label: string;
  state: ProtocolStageState;
  reason?: string;
}

export interface ProtocolWorkflow {
  id: string;
  number: string;
  employerName: string;
  programGroup: string;
  period: string;
  participants: number;
  stages: ProtocolStage[];
}

export interface Worker {
  id: string;
  fullName: string;
  snils: string;
  email: string;
  employerName: string;
  position: string;
  activeTrainings: number;
}

export interface Employer {
  id: string;
  name: string;
  inn: string;
  status: "active" | "inactive";
  requests: number;
  workers: number;
}

export interface Program {
  id: string;
  groupCode: string;
  code: string;
  name: string;
  defaultHours: number;
  status: "active" | "inactive";
  moodleCourseId?: string;
}

export interface GenerationRun {
  id: string;
  type: "xml" | "docx" | "xlsx" | "moodle_credentials";
  status: GenerationStatus;
  relatedEntity: string;
  fileName: string;
  generatedAt: string;
}

export interface MoodleAccount {
  id: string;
  workerName: string;
  employerName: string;
  email: string;
  status: MoodleStatus;
  course: string;
}

export interface AuditEvent {
  id: string;
  at: string;
  actor: string;
  action: string;
  entity: string;
  details: string;
}

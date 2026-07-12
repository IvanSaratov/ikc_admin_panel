import type { ProtocolWorkflow, WorkflowStage } from "../../api/mockProtocolWorkflow";

interface Props {
  workflow: ProtocolWorkflow;
}

export function ProtocolWorkflowPage({ workflow }: Props) {
  return (
    <div className="workflow-page">
      <header className="workflow-header">
        <div>
          <p className="eyebrow">Протокол</p>
          <h1>{workflow.number}</h1>
          <p>{workflow.employer}</p>
        </div>
        <button className="primary-button" type="button">
          Обновить статус
        </button>
      </header>

      <section className="pipeline" aria-label="Этапы протокола">
        {workflow.stages.map((stage) => (
          <StageCard key={stage.id} stage={stage} />
        ))}
      </section>
    </div>
  );
}

function StageCard({ stage }: { stage: WorkflowStage }) {
  return (
    <article className={`stage-card stage-card-${stage.state}`}>
      <span className="stage-state">{stage.state}</span>
      <h2>{stage.label}</h2>
      {stage.reason ? <p className="stage-reason">{stage.reason}</p> : null}
    </article>
  );
}

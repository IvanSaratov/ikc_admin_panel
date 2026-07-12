import { motion } from "motion/react";
import { useEffect, useState } from "react";
import { useParams } from "react-router";

import { getProtocolWorkflow } from "../../api/client";
import type { ProtocolStageState, ProtocolWorkflow } from "../../api/mock/types";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge, type StatusTone } from "../../components/status/StatusBadge";

function stageTone(state: ProtocolStageState): StatusTone {
  if (state === "done") {
    return "success";
  }
  if (state === "blocked") {
    return "danger";
  }
  if (state === "active") {
    return "warning";
  }
  return "neutral";
}

export function ProtocolDetailPage() {
  const params = useParams();
  const protocolId = params.protocolId ?? "protocol-2605-a-15";
  const [workflow, setWorkflow] = useState<ProtocolWorkflow>();
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const nextWorkflow = await getProtocolWorkflow(protocolId);
      setWorkflow(nextWorkflow);
    } catch {
      setWorkflow(undefined);
      setError(true);
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    let isMounted = true;

    async function load() {
      setIsLoading(true);
      setError(false);
      try {
        const nextWorkflow = await getProtocolWorkflow(protocolId);
        if (isMounted) {
          setWorkflow(nextWorkflow);
        }
      } catch {
        if (isMounted) {
          setWorkflow(undefined);
          setError(true);
        }
      } finally {
        if (isMounted) {
          setIsLoading(false);
        }
      }
    }

    void load();

    return () => {
      isMounted = false;
    };
  }, [protocolId]);

  if (isLoading) {
    return <LoadingState label="Загрузка протокола" />;
  }

  if (error || !workflow) {
    return (
      <ErrorState
        title="Не удалось загрузить протокол"
        description="Проверьте адрес протокола или повторите загрузку."
        actionLabel="Повторить"
        onAction={() => {
          void retry();
        }}
      />
    );
  }

  return (
    <div className="page-stack">
      <PageHeader
        title={`Протокол ${workflow.number}`}
        description={`${workflow.employerName}, группа ${workflow.programGroup}`}
      />
      <section className="protocol-stage-grid" aria-label="Этапы протокола">
        {workflow.stages.map((stage, index) => (
          <motion.article
            className={`protocol-stage protocol-stage-${stage.state}`}
            key={stage.id}
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.18, delay: index * 0.02, ease: "easeOut" }}
          >
            <div className="protocol-stage-header">
              <h2>{stage.label}</h2>
              <StatusBadge label={stage.state} tone={stageTone(stage.state)} />
            </div>
            {stage.reason ? <p>{stage.reason}</p> : null}
          </motion.article>
        ))}
      </section>
    </div>
  );
}

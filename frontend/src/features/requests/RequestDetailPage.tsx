import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";

import { listRequests } from "../../api/client";
import type { ClientRequest } from "../../api/mock/types";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge, type StatusTone } from "../../components/status/StatusBadge";

function statusTone(status: string): StatusTone {
  if (["ready", "completed"].includes(status)) {
    return "success";
  }
  if (status === "cancelled") {
    return "danger";
  }
  return "warning";
}

export function RequestDetailPage() {
  const params = useParams();
  const requestId = params.requestId ?? "request-1";
  const [request, setRequest] = useState<ClientRequest>();
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const requests = await listRequests();
      const nextRequest = requests.find((item) => item.id === requestId);
      if (!nextRequest) {
        setError(true);
        setRequest(undefined);
        return;
      }
      setRequest(nextRequest);
    } catch {
      setError(true);
      setRequest(undefined);
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
        const requests = await listRequests();
        const nextRequest = requests.find((item) => item.id === requestId);
        if (isMounted) {
          setRequest(nextRequest);
          setError(!nextRequest);
        }
      } catch {
        if (isMounted) {
          setRequest(undefined);
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
  }, [requestId]);

  if (isLoading) {
    return <LoadingState label="Загрузка заявки" />;
  }

  if (error || !request) {
    return (
      <ErrorState
        title="Заявка не найдена"
        description="Проверьте адрес карточки или повторите загрузку."
        actionLabel="Повторить"
        onAction={() => {
          void retry();
        }}
      />
    );
  }

  return (
    <div className="page-stack">
      <PageHeader title="Карточка заявки" description={`${request.employerName}, ${request.receivedDate}`} />

      <section className="panel" aria-label="Сводка заявки">
        <div className="panel-header">
          <h2>{request.id}</h2>
          <StatusBadge label={request.status} tone={statusTone(request.status)} />
        </div>
        <div className="panel-body operation-summary">
          <span>Работодатель: {request.employerName}</span>
          <span>Строк всего: {request.rowsTotal}</span>
          <span>На проверку: {request.rowsNeedReview}</span>
          <span>Следующий шаг: {request.nextAction}</span>
        </div>
      </section>

      <div className="operation-link-grid">
        <Link className="operation-link-panel" to="/imports/import-1">
          <strong>Разбор импорта</strong>
          <span>Открыть staging rows и конфликты.</span>
        </Link>
        <Link className="operation-link-panel" to="/protocols/protocol-2605-a-15">
          <strong>Протокол 2605А15</strong>
          <span>Проверить workflow и gate-причины.</span>
        </Link>
      </div>
    </div>
  );
}

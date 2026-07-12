import { AlertTriangle, ClipboardList, FileText, UploadCloud } from "lucide-react";
import { useEffect, useState } from "react";

import { listRequests } from "../../api/client";
import type { ClientRequest } from "../../api/mock/types";
import { EmptyState } from "../../components/feedback/EmptyState";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge, type StatusTone } from "../../components/status/StatusBadge";

const metrics = [
  { label: "Заявки в review", value: "14", icon: ClipboardList, tone: "neutral" },
  { label: "Конфликты импорта", value: "7", icon: UploadCloud, tone: "warning" },
  { label: "DOCX blocked", value: "3", icon: AlertTriangle, tone: "danger" },
  { label: "Следующее действие", value: "XML", icon: FileText, tone: "neutral" }
] as const;

function getAttentionTone(request: ClientRequest): StatusTone {
  return request.attention === "danger" ? "danger" : "warning";
}

interface DashboardPageProps {
  loadRequests?: () => Promise<ClientRequest[]>;
}

export function DashboardPage({ loadRequests = listRequests }: DashboardPageProps) {
  const [requests, setRequests] = useState<ClientRequest[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    let isMounted = true;

    async function load() {
      setIsLoading(true);
      setError(false);
      try {
        const nextRequests = await loadRequests();
        if (isMounted) {
          setRequests(nextRequests);
        }
      } catch {
        if (isMounted) {
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
  }, [loadRequests]);

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const nextRequests = await loadRequests();
      setRequests(nextRequests);
    } catch {
      setError(true);
    } finally {
      setIsLoading(false);
    }
  }

  if (isLoading) {
    return <LoadingState label="Загрузка рабочего стола" />;
  }

  if (error) {
    return (
      <ErrorState
        title="Не удалось загрузить рабочий стол"
        description="Проверьте подключение к данным и повторите попытку."
        actionLabel="Повторить"
        onAction={() => {
          void retry();
        }}
      />
    );
  }

  const attention = requests.filter((request) => request.attention !== "normal");

  return (
    <div className="page-stack">
      <PageHeader
        eyebrow="Операции"
        title="Рабочий стол"
        description="Очереди, блокировки и быстрые переходы по Минтруд-процессу."
        actions={<StatusBadge label="Mock data" tone="info" />}
      />

      <section className="metric-grid" aria-label="Операционные метрики">
        {metrics.map((metric) => {
          const Icon = metric.icon;
          return (
            <article className={`metric-tile metric-tile-${metric.tone}`} key={metric.label}>
              <Icon className="metric-icon" aria-hidden />
              <div>
                <p>{metric.label}</p>
                <strong>{metric.value}</strong>
              </div>
            </article>
          );
        })}
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Требуют внимания</h2>
          <StatusBadge
            label={`${attention.length} задачи`}
            tone={attention.length > 0 ? "warning" : "success"}
          />
        </div>

        {attention.length === 0 ? (
          <EmptyState
            title="Очередь пуста"
            description="Сейчас нет заявок с предупреждениями или блокировками."
          />
        ) : (
          <div className="attention-list">
            {attention.map((request) => (
              <article className="attention-item" key={request.id}>
                <div>
                  <h3>{request.employerName}</h3>
                  <p>{request.nextAction}</p>
                </div>
                <StatusBadge label={request.attention} tone={getAttentionTone(request)} />
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

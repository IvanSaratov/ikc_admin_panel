import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useState } from "react";

import { listAuditEvents } from "../../api/client";
import type { AuditEvent } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";

const columns: ColumnDef<AuditEvent>[] = [
  {
    accessorKey: "at",
    header: "Время",
  },
  {
    accessorKey: "actor",
    header: "Actor",
  },
  {
    accessorKey: "action",
    header: "Action",
  },
  {
    accessorKey: "entity",
    header: "Entity",
  },
  {
    accessorKey: "details",
    header: "Details",
  },
];

export function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const nextEvents = await listAuditEvents();
      setEvents(nextEvents);
    } catch {
      setError(true);
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    let isMounted = true;

    async function load() {
      try {
        const nextEvents = await listAuditEvents();
        if (isMounted) {
          setEvents(nextEvents);
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
  }, []);

  if (isLoading) {
    return <LoadingState label="Загрузка журнала" />;
  }

  if (error) {
    return (
      <ErrorState
        title="Не удалось загрузить журнал"
        description="Проверьте подключение к данным и повторите попытку."
        actionLabel="Повторить"
        onAction={() => {
          void retry();
        }}
      />
    );
  }

  return (
    <div className="page-stack">
      <PageHeader title="Журнал" description="Действия оператора и системные события." />
      <DataTable ariaLabel="Журнал" data={events} columns={columns} />
    </div>
  );
}

import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useState } from "react";
import { Link } from "react-router";

import { listRequests } from "../../api/client";
import type { ClientRequest } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge, type StatusTone } from "../../components/status/StatusBadge";

function statusTone(status: string): StatusTone {
  if (["success", "enrolled", "imported", "active", "completed", "ready"].includes(status)) {
    return "success";
  }
  if (["failed", "conflict", "invalid", "cancelled"].includes(status)) {
    return "danger";
  }
  if (["review_required", "needs_rebuild", "running", "review"].includes(status)) {
    return "warning";
  }
  return "info";
}

const columns: ColumnDef<ClientRequest>[] = [
  {
    accessorKey: "employerName",
    header: "Работодатель",
    cell: ({ row }) => <Link to={`/requests/${row.original.id}`}>{row.original.employerName}</Link>,
  },
  {
    accessorKey: "receivedDate",
    header: "Дата",
  },
  {
    accessorKey: "status",
    header: "Статус",
    cell: ({ row }) => <StatusBadge label={row.original.status} tone={statusTone(row.original.status)} />,
  },
  {
    accessorKey: "rowsNeedReview",
    header: "На проверку",
  },
  {
    accessorKey: "nextAction",
    header: "Следующий шаг",
  },
];

export function RequestsPage() {
  const [requests, setRequests] = useState<ClientRequest[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const nextRequests = await listRequests();
      setRequests(nextRequests);
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
        const nextRequests = await listRequests();
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
  }, []);

  if (isLoading) {
    return <LoadingState label="Загрузка заявок" />;
  }

  if (error) {
    return (
      <ErrorState
        title="Не удалось загрузить заявки"
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
      <PageHeader title="Заявки" description="Клиентские заявки и следующий операторский шаг." />
      <DataTable ariaLabel="Заявки" data={requests} columns={columns} />
    </div>
  );
}

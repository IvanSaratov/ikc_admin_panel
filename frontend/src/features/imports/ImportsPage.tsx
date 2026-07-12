import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useState } from "react";
import { Link } from "react-router";

import { listImportRuns } from "../../api/client";
import type { ImportRun } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge, type StatusTone } from "../../components/status/StatusBadge";

function statusTone(status: string): StatusTone {
  if (["success", "enrolled", "imported", "active", "completed"].includes(status)) {
    return "success";
  }
  if (["failed", "conflict", "invalid"].includes(status)) {
    return "danger";
  }
  if (["review_required", "needs_rebuild", "running", "review"].includes(status)) {
    return "warning";
  }
  return "info";
}

const columns: ColumnDef<ImportRun>[] = [
  {
    accessorKey: "fileName",
    header: "Файл",
    cell: ({ row }) => <Link to={`/imports/${row.original.id}`}>{row.original.fileName}</Link>,
  },
  {
    accessorKey: "requestId",
    header: "Заявка",
  },
  {
    accessorKey: "status",
    header: "Статус",
    cell: ({ row }) => <StatusBadge label={row.original.status} tone={statusTone(row.original.status)} />,
  },
  {
    accessorKey: "rowsTotal",
    header: "Строк",
  },
];

export function ImportsPage() {
  const [imports, setImports] = useState<ImportRun[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const nextImports = await listImportRuns();
      setImports(nextImports);
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
        const nextImports = await listImportRuns();
        if (isMounted) {
          setImports(nextImports);
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
    return <LoadingState label="Загрузка импортов" />;
  }

  if (error) {
    return (
      <ErrorState
        title="Не удалось загрузить импорты"
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
      <PageHeader title="Импорт" description="XLSX-загрузки, staging rows, дубли и конфликты." />
      <DataTable ariaLabel="Импорт" data={imports} columns={columns} />
    </div>
  );
}

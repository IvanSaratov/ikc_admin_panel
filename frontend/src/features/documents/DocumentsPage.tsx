import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useMemo, useState } from "react";

import { listGenerationRuns } from "../../api/client";
import type { GenerationRun } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge, type StatusTone } from "../../components/status/StatusBadge";

function statusTone(status: string): StatusTone {
  if (["success", "enrolled", "imported", "active"].includes(status)) {
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

export function DocumentsPage() {
  const [runs, setRuns] = useState<GenerationRun[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);
  const columns = useMemo<ColumnDef<GenerationRun>[]>(
    () => [
      {
        accessorKey: "type",
        header: "Тип",
      },
      {
        accessorKey: "status",
        header: "Статус",
        cell: ({ row }) => <StatusBadge label={row.original.status} tone={statusTone(row.original.status)} />,
      },
      {
        accessorKey: "relatedEntity",
        header: "Объект",
      },
      {
        accessorKey: "fileName",
        header: "Файл",
      },
      {
        accessorKey: "generatedAt",
        header: "Создан",
      },
      {
        id: "download",
        header: "Скачать",
        enableSorting: false,
        cell: ({ row }) => (
          <button className="button button-secondary" type="button">
            Скачать {row.original.fileName}
          </button>
        ),
      },
    ],
    [],
  );

  useEffect(() => {
    let isMounted = true;

    async function load() {
      try {
        const nextRuns = await listGenerationRuns();
        if (isMounted) {
          setRuns(nextRuns);
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

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const nextRuns = await listGenerationRuns();
      setRuns(nextRuns);
    } catch {
      setError(true);
    } finally {
      setIsLoading(false);
    }
  }

  if (isLoading) {
    return <LoadingState label="Загрузка документов" />;
  }

  if (error) {
    return (
      <ErrorState
        title="Не удалось загрузить документы"
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
      <PageHeader title="Документы" description="История генерации XML, DOCX и XLSX." />
      <DataTable ariaLabel="Документы" data={runs} columns={columns} />
    </div>
  );
}

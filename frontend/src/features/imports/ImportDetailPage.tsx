import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router";

import { listImportRows, resolveImportRow } from "../../api/client";
import type { ImportRow } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge } from "../../components/status/StatusBadge";

export function ImportDetailPage() {
  const params = useParams();
  const importId = params.importId ?? "import-1";
  const [rows, setRows] = useState<ImportRow[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  async function refresh() {
    setIsLoading(true);
    setError(false);
    try {
      const nextRows = await listImportRows(importId);
      setRows(nextRows);
    } catch {
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
        const nextRows = await listImportRows(importId);
        if (isMounted) {
          setRows(nextRows);
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
  }, [importId]);

  const columns = useMemo<ColumnDef<ImportRow>[]>(
    () => [
      {
        accessorKey: "rowNumber",
        header: "Строка",
      },
      {
        accessorKey: "fullName",
        header: "ФИО",
      },
      {
        accessorKey: "snils",
        header: "СНИЛС",
      },
      {
        accessorKey: "programs",
        header: "Программы",
        cell: ({ row }) => row.original.programs.join(", "),
      },
      {
        accessorKey: "status",
        header: "Статус",
        cell: ({ row }) => (
          <StatusBadge
            label={row.original.status}
            tone={row.original.status === "conflict" ? "danger" : "info"}
          />
        ),
      },
      {
        id: "actions",
        header: "Действия",
        enableSorting: false,
        cell: ({ row }) => (
          <button
            className="button button-secondary"
            type="button"
            onClick={() => {
              void resolveImportRow(row.original.id, "skipped")
                .then(refresh)
                .catch(() => {
                  setError(true);
                });
            }}
          >
            Пропустить {row.original.id}
          </button>
        ),
      },
    ],
    [refresh],
  );

  if (isLoading) {
    return <LoadingState label="Загрузка строк импорта" />;
  }

  if (error) {
    return (
      <ErrorState
        title="Не удалось загрузить импорт"
        description="Проверьте подключение к данным и повторите попытку."
        actionLabel="Повторить"
        onAction={() => {
          void refresh();
        }}
      />
    );
  }

  return (
    <div className="page-stack">
      <PageHeader
        title="Разбор импорта"
        description="Staging rows, конфликты и решения оператора."
      />
      <DataTable ariaLabel="Строки импорта" data={rows} columns={columns} />
    </div>
  );
}

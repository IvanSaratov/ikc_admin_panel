import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useState } from "react";

import { listMoodleAccounts } from "../../api/client";
import type { MoodleAccount } from "../../api/mock/types";
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

const columns: ColumnDef<MoodleAccount>[] = [
  {
    accessorKey: "workerName",
    header: "Слушатель",
  },
  {
    accessorKey: "employerName",
    header: "Работодатель",
  },
  {
    accessorKey: "email",
    header: "Email",
  },
  {
    accessorKey: "course",
    header: "Курс",
  },
  {
    accessorKey: "status",
    header: "Статус",
    cell: ({ row }) => <StatusBadge label={row.original.status} tone={statusTone(row.original.status)} />,
  },
];

export function MoodlePage() {
  const [accounts, setAccounts] = useState<MoodleAccount[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const nextAccounts = await listMoodleAccounts();
      setAccounts(nextAccounts);
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
        const nextAccounts = await listMoodleAccounts();
        if (isMounted) {
          setAccounts(nextAccounts);
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
    return <LoadingState label="Загрузка Moodle" />;
  }

  if (error) {
    return (
      <ErrorState
        title="Не удалось загрузить Moodle"
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
      <PageHeader title="Moodle" description="Зачисления, аккаунты и файл учетных данных." />
      <DataTable ariaLabel="Moodle" data={accounts} columns={columns} />
    </div>
  );
}

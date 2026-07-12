import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useState } from "react";
import { Link } from "react-router";

import { listProtocols } from "../../api/client";
import type { ProtocolWorkflow } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { ErrorState } from "../../components/feedback/ErrorState";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";

const columns: ColumnDef<ProtocolWorkflow>[] = [
  {
    accessorKey: "number",
    header: "Номер",
    cell: ({ row }) => <Link to={`/protocols/${row.original.id}`}>{row.original.number}</Link>,
  },
  {
    accessorKey: "employerName",
    header: "Работодатель",
  },
  {
    accessorKey: "programGroup",
    header: "Группа",
  },
  {
    accessorKey: "period",
    header: "Период",
  },
  {
    accessorKey: "participants",
    header: "Участники",
  },
];

export function ProtocolsPage() {
  const [protocols, setProtocols] = useState<ProtocolWorkflow[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(false);

  async function retry() {
    setIsLoading(true);
    setError(false);
    try {
      const nextProtocols = await listProtocols();
      setProtocols(nextProtocols);
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
        const nextProtocols = await listProtocols();
        if (isMounted) {
          setProtocols(nextProtocols);
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
    return <LoadingState label="Загрузка протоколов" />;
  }

  if (error) {
    return (
      <ErrorState
        title="Не удалось загрузить протоколы"
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
      <PageHeader title="Протоколы" description="Протоколы, статусы XML/DOCX и gate-причины." />
      <DataTable ariaLabel="Протоколы" data={protocols} columns={columns} />
    </div>
  );
}

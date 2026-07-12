import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useState } from "react";
import { Link } from "react-router";

import { listEmployers } from "../../api/client";
import type { Employer } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge } from "../../components/status/StatusBadge";

const columns: ColumnDef<Employer>[] = [
  {
    accessorKey: "name",
    header: "Название",
    cell: ({ row }) => <Link to={`/employers/${row.original.id}`}>{row.original.name}</Link>,
  },
  {
    accessorKey: "inn",
    header: "ИНН",
  },
  {
    accessorKey: "status",
    header: "Статус",
    cell: ({ row }) => (
      <StatusBadge
        label={row.original.status}
        tone={row.original.status === "active" ? "success" : "neutral"}
      />
    ),
  },
  {
    accessorKey: "requests",
    header: "Заявки",
  },
  {
    accessorKey: "workers",
    header: "Слушатели",
  },
];

export function EmployersPage() {
  const [employers, setEmployers] = useState<Employer[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let isMounted = true;

    async function load() {
      const nextEmployers = await listEmployers();
      if (isMounted) {
        setEmployers(nextEmployers);
        setIsLoading(false);
      }
    }

    void load();

    return () => {
      isMounted = false;
    };
  }, []);

  if (isLoading) {
    return <LoadingState label="Загрузка работодателей" />;
  }

  return (
    <div className="page-stack">
      <PageHeader title="Работодатели" description="Компании, ИНН, заявки и связанные слушатели." />
      <DataTable ariaLabel="Работодатели" data={employers} columns={columns} />
    </div>
  );
}

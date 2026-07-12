import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useState } from "react";
import { Link } from "react-router";

import { listWorkers } from "../../api/client";
import type { Worker } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";

const columns: ColumnDef<Worker>[] = [
  {
    accessorKey: "fullName",
    header: "ФИО",
    cell: ({ row }) => <Link to={`/workers/${row.original.id}`}>{row.original.fullName}</Link>,
  },
  {
    accessorKey: "snils",
    header: "СНИЛС",
  },
  {
    accessorKey: "email",
    header: "Email",
  },
  {
    accessorKey: "employerName",
    header: "Работодатель",
  },
  {
    accessorKey: "activeTrainings",
    header: "Активные обучения",
  },
];

export function WorkersPage() {
  const [workers, setWorkers] = useState<Worker[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let isMounted = true;

    async function load() {
      const nextWorkers = await listWorkers();
      if (isMounted) {
        setWorkers(nextWorkers);
        setIsLoading(false);
      }
    }

    void load();

    return () => {
      isMounted = false;
    };
  }, []);

  if (isLoading) {
    return <LoadingState label="Загрузка слушателей" />;
  }

  return (
    <div className="page-stack">
      <PageHeader title="Слушатели" description="Физлица, работодатели и активные обучения." />
      <DataTable ariaLabel="Слушатели" data={workers} columns={columns} />
    </div>
  );
}

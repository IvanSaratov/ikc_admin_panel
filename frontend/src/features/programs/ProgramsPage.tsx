import type { ColumnDef } from "@tanstack/react-table";
import { useEffect, useState } from "react";

import { listPrograms } from "../../api/client";
import type { Program } from "../../api/mock/types";
import { DataTable } from "../../components/data-table/DataTable";
import { LoadingState } from "../../components/feedback/LoadingState";
import { PageHeader } from "../../components/layout/PageHeader";
import { StatusBadge } from "../../components/status/StatusBadge";

const columns: ColumnDef<Program>[] = [
  {
    accessorKey: "groupCode",
    header: "Группа",
  },
  {
    accessorKey: "code",
    header: "Код",
  },
  {
    accessorKey: "name",
    header: "Название",
  },
  {
    accessorKey: "defaultHours",
    header: "Часы",
  },
  {
    accessorKey: "moodleCourseId",
    header: "Moodle course",
    cell: ({ row }) => row.original.moodleCourseId ?? "—",
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
];

export function ProgramsPage() {
  const [programs, setPrograms] = useState<Program[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let isMounted = true;

    async function load() {
      const nextPrograms = await listPrograms();
      if (isMounted) {
        setPrograms(nextPrograms);
        setIsLoading(false);
      }
    }

    void load();

    return () => {
      isMounted = false;
    };
  }, []);

  if (isLoading) {
    return <LoadingState label="Загрузка программ" />;
  }

  return (
    <div className="page-stack">
      <PageHeader title="Программы" description="Группы, часы, статус и Moodle mapping." />
      <DataTable ariaLabel="Программы" data={programs} columns={columns} />
    </div>
  );
}

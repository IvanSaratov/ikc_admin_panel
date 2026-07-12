import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { useMemo, useState } from "react";

interface DataTableProps<TData extends object> {
  ariaLabel: string;
  data: TData[];
  columns: ColumnDef<TData, unknown>[];
  emptyMessage?: string;
}

function ariaSortValue(sortState: false | "asc" | "desc") {
  if (sortState === "asc") {
    return "ascending";
  }
  if (sortState === "desc") {
    return "descending";
  }
  return "none";
}

export function DataTable<TData extends object>({
  ariaLabel,
  data,
  columns,
  emptyMessage = "Нет строк для отображения",
}: DataTableProps<TData>) {
  const [globalFilter, setGlobalFilter] = useState("");
  const [sorting, setSorting] = useState<SortingState>([]);
  const stableData = useMemo(() => data, [data]);
  const table = useReactTable({
    data: stableData,
    columns,
    state: { globalFilter, sorting },
    onGlobalFilterChange: setGlobalFilter,
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <section className="data-table-panel">
      <div className="data-table-tools">
        <input
          aria-label="Фильтр таблицы"
          type="search"
          value={globalFilter}
          onChange={(event) => setGlobalFilter(event.target.value)}
          placeholder="Фильтр"
        />
      </div>
      <div className="data-table-scroll">
        <table className="data-table" aria-label={ariaLabel}>
          <thead>
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <th
                    key={header.id}
                    aria-sort={header.column.getCanSort() ? ariaSortValue(header.column.getIsSorted()) : undefined}
                  >
                    {header.isPlaceholder
                      ? null
                      : header.column.getCanSort()
                        ? (
                            <button
                              className="table-sort-button"
                              type="button"
                              onClick={header.column.getToggleSortingHandler()}
                            >
                              {flexRender(header.column.columnDef.header, header.getContext())}
                            </button>
                          )
                        : flexRender(header.column.columnDef.header, header.getContext())}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.length > 0 ? (
              table.getRowModel().rows.map((row) => (
                <tr key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>
                  ))}
                </tr>
              ))
            ) : (
              <tr>
                <td className="data-table-empty" colSpan={table.getAllLeafColumns().length}>
                  {emptyMessage}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

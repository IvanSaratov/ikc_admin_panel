import { Search } from "lucide-react";

export function TopBar() {
  return (
    <header className="topbar">
      <label className="topbar-search">
        <Search className="topbar-search-icon" aria-hidden />
        <span className="sr-only">Поиск по админке</span>
        <input aria-label="Поиск по админке" type="search" placeholder="Поиск по админке" />
      </label>
      <div className="topbar-status" aria-label="Статус окружения">
        <span className="status-pill status-pill-warning">Mock data</span>
        <span className="status-pill">operator</span>
      </div>
    </header>
  );
}

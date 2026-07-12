import { PageHeader } from "../../components/layout/PageHeader";

export function SettingsPage() {
  return (
    <div className="page-stack">
      <PageHeader title="Настройки" description="Шаблоны, backup, режим моков и системная информация." />

      <section className="panel">
        <div className="panel-header">
          <h2>Режим данных</h2>
        </div>
        <div className="panel-body">Mock data</div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Backup SQLite</h2>
        </div>
        <div className="panel-body">ежедневно, mock status</div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Шаблоны</h2>
        </div>
        <div className="panel-body">
          <button className="button button-secondary" type="button">
            Client XLSX template
          </button>{" "}
          <button className="button button-secondary" type="button">
            Protocol example
          </button>
        </div>
      </section>

      <section className="panel">
        <div className="panel-header">
          <h2>Системная информация</h2>
        </div>
        <div className="panel-body">frontend-shell-mock</div>
      </section>
    </div>
  );
}

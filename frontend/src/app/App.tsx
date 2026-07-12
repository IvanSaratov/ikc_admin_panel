import { mockProtocolWorkflow } from "../api/mockProtocolWorkflow";
import { ProtocolWorkflowPage } from "../features/protocol-workflow/ProtocolWorkflowPage";

export function App() {
  return (
    <main className="app-shell">
      <aside className="app-nav">
        <strong>Mintrud Admin</strong>
        <a href="/protocols">Протоколы</a>
        <a href="/requests">Заявки</a>
        <a href="/workers">Слушатели</a>
      </aside>
      <section className="app-content">
        <ProtocolWorkflowPage workflow={mockProtocolWorkflow} />
      </section>
    </main>
  );
}

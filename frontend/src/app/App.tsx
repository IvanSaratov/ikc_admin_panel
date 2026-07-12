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
        <h1>Protocol workflow</h1>
        <p>React frontend is ready for the first workflow slice.</p>
      </section>
    </main>
  );
}

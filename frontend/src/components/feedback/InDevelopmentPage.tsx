interface InDevelopmentPageProps {
  title: string;
  description: string;
  planned: string[];
}

export function InDevelopmentPage({ title, description, planned }: InDevelopmentPageProps) {
  return (
    <section className="in-dev-page">
      <p className="eyebrow">Сейчас в разработке</p>
      <h1>{title}</h1>
      <p>{description}</p>
      <div className="in-dev-panel">
        <h2>Планируемые возможности</h2>
        <ul>
          {planned.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </div>
    </section>
  );
}

import { useParams } from "react-router";

import { PageHeader } from "../../components/layout/PageHeader";

export function EmployerDetailPage() {
  const { employerId } = useParams();

  return (
    <div className="page-stack">
      <PageHeader title="Карточка работодателя" description={`Mock detail route: ${employerId}`} />
      <section className="panel">
        <div className="panel-header">
          <h2>Связанные данные</h2>
        </div>
        <div className="panel-body">
          В API-этапе здесь появятся заявки, слушатели и протоколы работодателя.
        </div>
      </section>
    </div>
  );
}

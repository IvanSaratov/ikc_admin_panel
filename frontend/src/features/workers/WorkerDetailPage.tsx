import { useParams } from "react-router";

import { PageHeader } from "../../components/layout/PageHeader";

export function WorkerDetailPage() {
  const { workerId } = useParams();

  return (
    <div className="page-stack">
      <PageHeader title="Карточка слушателя" description={`Mock detail route: ${workerId}`} />
      <section className="panel">
        <div className="panel-header">
          <h2>Связанные данные</h2>
        </div>
        <div className="panel-body">
          В API-этапе здесь появятся обучения, работодатели, Moodle и протоколы слушателя.
        </div>
      </section>
    </div>
  );
}

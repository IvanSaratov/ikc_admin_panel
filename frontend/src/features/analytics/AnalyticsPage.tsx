import { InDevelopmentPage } from "../../components/feedback/InDevelopmentPage";

export function AnalyticsPage() {
  return (
    <InDevelopmentPage
      title="Аналитика"
      description="Раздел появится после стабилизации MVP workflow."
      planned={["Динамика заявок", "Статусы протоколов", "Ошибки импортов", "Moodle-очередь"]}
    />
  );
}

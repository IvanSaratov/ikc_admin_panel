import { InDevelopmentPage } from "../../components/feedback/InDevelopmentPage";

export function NotificationsPage() {
  return (
    <InDevelopmentPage
      title="Уведомления"
      description="Внешние уведомления не входят в MVP и будут подключаться по подтвержденным сценариям."
      planned={["Email об ошибках", "Telegram для критичных событий", "Уведомления о backup", "Ошибки Moodle"]}
    />
  );
}

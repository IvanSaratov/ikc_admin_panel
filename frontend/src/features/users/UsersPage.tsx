import { InDevelopmentPage } from "../../components/feedback/InDevelopmentPage";

export function UsersPage() {
  return (
    <InDevelopmentPage
      title="Пользователи и роли"
      description="RBAC появится после MVP, когда будет подтвержден состав ролей."
      planned={["Операторы", "Роли доступа", "Персональный audit", "Блокировка пользователей"]}
    />
  );
}

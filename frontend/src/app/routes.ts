import {
  BarChart3,
  Bell,
  BookOpen,
  BriefcaseBusiness,
  ClipboardList,
  FileArchive,
  FileText,
  GraduationCap,
  History,
  Home,
  Settings,
  ShieldCheck,
  UploadCloud,
  Users,
} from "lucide-react";
import type { ComponentType } from "react";

export type RouteId =
  | "dashboard"
  | "requests"
  | "requestDetail"
  | "imports"
  | "importDetail"
  | "protocols"
  | "protocolDetail"
  | "documents"
  | "moodle"
  | "workers"
  | "workerDetail"
  | "employers"
  | "employerDetail"
  | "programs"
  | "audit"
  | "analytics"
  | "users"
  | "notifications"
  | "settings";

export interface AppRoute {
  id: RouteId;
  path: string;
  label: string;
  description: string;
  icon: ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  group?: "operations" | "registry" | "control" | "admin";
  inDevelopment?: boolean;
}

export const appRoutes: AppRoute[] = [
  {
    id: "dashboard",
    path: "/dashboard",
    label: "Рабочий стол",
    description: "Операционный обзор заявок, протоколов и очередей внимания.",
    icon: Home,
  },
  {
    id: "requests",
    path: "/requests",
    label: "Заявки",
    description: "Клиентские заявки и их текущий статус.",
    icon: ClipboardList,
    group: "operations",
  },
  {
    id: "requestDetail",
    path: "/requests/:requestId",
    label: "Карточка заявки",
    description: "Детали заявки и связанные строки импорта.",
    icon: ClipboardList,
    group: "operations",
  },
  {
    id: "imports",
    path: "/imports",
    label: "Импорт",
    description: "XLSX-загрузки, staging rows, дубли и конфликты.",
    icon: UploadCloud,
    group: "operations",
  },
  {
    id: "importDetail",
    path: "/imports/:importId",
    label: "Разбор импорта",
    description: "Проверка строк, конфликтов и решений оператора.",
    icon: UploadCloud,
    group: "operations",
  },
  {
    id: "protocols",
    path: "/protocols",
    label: "Протоколы",
    description: "Протоколы, статусы XML/DOCX и gate-причины.",
    icon: FileText,
    group: "operations",
  },
  {
    id: "protocolDetail",
    path: "/protocols/:protocolId",
    label: "Карточка протокола",
    description: "Workflow протокола, участники и документы.",
    icon: FileText,
    group: "operations",
  },
  {
    id: "documents",
    path: "/documents",
    label: "Документы",
    description: "История генерации XML, DOCX и XLSX.",
    icon: FileArchive,
    group: "operations",
  },
  {
    id: "moodle",
    path: "/moodle",
    label: "Moodle",
    description: "Зачисления, аккаунты и файл учетных данных.",
    icon: GraduationCap,
    group: "operations",
  },
  {
    id: "workers",
    path: "/workers",
    label: "Слушатели",
    description: "Реестр физических лиц и их обучений.",
    icon: Users,
    group: "registry",
  },
  {
    id: "workerDetail",
    path: "/workers/:workerId",
    label: "Карточка слушателя",
    description: "Данные слушателя, работодатели и обучения.",
    icon: Users,
    group: "registry",
  },
  {
    id: "employers",
    path: "/employers",
    label: "Работодатели",
    description: "Организации, ИНН, заявки и слушатели.",
    icon: BriefcaseBusiness,
    group: "registry",
  },
  {
    id: "employerDetail",
    path: "/employers/:employerId",
    label: "Карточка работодателя",
    description: "Компания, связанные заявки и слушатели.",
    icon: BriefcaseBusiness,
    group: "registry",
  },
  {
    id: "programs",
    path: "/programs",
    label: "Программы",
    description: "Группы программ, часы и Moodle mapping.",
    icon: BookOpen,
    group: "registry",
  },
  {
    id: "audit",
    path: "/audit",
    label: "Журнал",
    description: "Действия оператора и системные события.",
    icon: History,
    group: "control",
  },
  {
    id: "analytics",
    path: "/analytics",
    label: "Аналитика",
    description: "Будущие управленческие метрики.",
    icon: BarChart3,
    group: "control",
    inDevelopment: true,
  },
  {
    id: "users",
    path: "/users",
    label: "Пользователи и роли",
    description: "Будущая RBAC-модель.",
    icon: ShieldCheck,
    group: "admin",
    inDevelopment: true,
  },
  {
    id: "notifications",
    path: "/notifications",
    label: "Уведомления",
    description: "Будущие email и Telegram уведомления.",
    icon: Bell,
    group: "admin",
    inDevelopment: true,
  },
  {
    id: "settings",
    path: "/settings",
    label: "Настройки",
    description: "Шаблоны, backup, режим моков и системная информация.",
    icon: Settings,
    group: "admin",
  },
];

export const routeIds = appRoutes.map((route) => route.id);

export const routesByPath = Object.fromEntries(appRoutes.map((route) => [route.path, route])) as Partial<Record<
  string,
  AppRoute
>>;

function isPrimaryNavRoute(route: AppRoute) {
  return !route.path.includes(":");
}

export const navGroups = [
  {
    id: "operations",
    label: "Операции",
    items: appRoutes.filter((route) => route.group === "operations" && isPrimaryNavRoute(route)),
  },
  {
    id: "registry",
    label: "Реестр",
    items: appRoutes.filter((route) => route.group === "registry" && isPrimaryNavRoute(route)),
  },
  {
    id: "control",
    label: "Контроль",
    items: appRoutes.filter((route) => route.group === "control" && isPrimaryNavRoute(route)),
  },
  {
    id: "admin",
    label: "Администрирование",
    items: appRoutes.filter((route) => route.group === "admin" && isPrimaryNavRoute(route)),
  },
] as const;

export const dashboardRoute = appRoutes.find((route) => route.id === "dashboard")!;

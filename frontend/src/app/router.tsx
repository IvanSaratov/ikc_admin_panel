import { createBrowserRouter, Navigate, useNavigate } from "react-router";
import type { ReactElement } from "react";

import { login } from "../api/client";
import { InDevelopmentPage } from "../components/feedback/InDevelopmentPage";
import { AnalyticsPage } from "../features/analytics/AnalyticsPage";
import { LoginPage } from "../features/auth/LoginPage";
import { AuditPage } from "../features/audit/AuditPage";
import { DashboardPage } from "../features/dashboard/DashboardPage";
import { DocumentsPage } from "../features/documents/DocumentsPage";
import { EmployerDetailPage } from "../features/employers/EmployerDetailPage";
import { EmployersPage } from "../features/employers/EmployersPage";
import { ImportDetailPage } from "../features/imports/ImportDetailPage";
import { ImportsPage } from "../features/imports/ImportsPage";
import { MoodlePage } from "../features/moodle/MoodlePage";
import { NotificationsPage } from "../features/notifications/NotificationsPage";
import { ProgramsPage } from "../features/programs/ProgramsPage";
import { ProtocolDetailPage } from "../features/protocols/ProtocolDetailPage";
import { ProtocolsPage } from "../features/protocols/ProtocolsPage";
import { RequestDetailPage } from "../features/requests/RequestDetailPage";
import { RequestsPage } from "../features/requests/RequestsPage";
import { SettingsPage } from "../features/settings/SettingsPage";
import { UsersPage } from "../features/users/UsersPage";
import { WorkerDetailPage } from "../features/workers/WorkerDetailPage";
import { WorkersPage } from "../features/workers/WorkersPage";
import { AppShell } from "./shell/AppShell";
import { appRoutes, type RouteId } from "./routes";

const planned = ["Макет таблиц", "Фильтры", "Детальная карточка"];

function PageStub({ title }: { title: string }) {
  return (
    <InDevelopmentPage
      title={title}
      description="Каркас раздела подключен к навигации."
      planned={planned}
    />
  );
}

const routeElements: Partial<Record<RouteId, ReactElement>> = {
  dashboard: <DashboardPage />,
  requests: <RequestsPage />,
  requestDetail: <RequestDetailPage />,
  imports: <ImportsPage />,
  importDetail: <ImportDetailPage />,
  protocols: <ProtocolsPage />,
  protocolDetail: <ProtocolDetailPage />,
  documents: <DocumentsPage />,
  moodle: <MoodlePage />,
  workers: <WorkersPage />,
  workerDetail: <WorkerDetailPage />,
  employers: <EmployersPage />,
  employerDetail: <EmployerDetailPage />,
  programs: <ProgramsPage />,
  audit: <AuditPage />,
  analytics: <AnalyticsPage />,
  users: <UsersPage />,
  notifications: <NotificationsPage />,
  settings: <SettingsPage />,
};

const shellRoutes = appRoutes.map((route) => ({
  path: route.path.replace(/^\//, ""),
  element: routeElements[route.id] ?? <PageStub title={route.label} />,
}));

function LoginRoute() {
  const navigate = useNavigate();

  return (
    <LoginPage
      onLogin={async (input) => {
        await login(input);
        await navigate("/dashboard", { replace: true });
      }}
    />
  );
}

export const router = createBrowserRouter([
  {
    path: "/login",
    element: <LoginRoute />,
  },
  {
    path: "/",
    element: <AppShell />,
    children: [
      {
        index: true,
        element: <Navigate to="/dashboard" replace />,
      },
      ...shellRoutes,
    ],
  },
]);

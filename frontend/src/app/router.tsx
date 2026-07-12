import { createBrowserRouter, Navigate } from "react-router";

import { InDevelopmentPage } from "../components/feedback/InDevelopmentPage";
import { AppShell } from "./shell/AppShell";
import { appRoutes } from "./routes";

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

const shellRoutes = appRoutes.map((route) => ({
  path: route.path.replace(/^\//, ""),
  element: <PageStub title={route.label} />,
}));

export const router = createBrowserRouter([
  {
    path: "/login",
    element: <PageStub title="Вход" />,
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

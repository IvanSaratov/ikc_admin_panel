import { ChevronDown, ChevronRight } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { NavLink, useLocation } from "react-router";

import { appRoutes, navGroups } from "../routes";

const dashboardRoute = appRoutes.find((route) => route.id === "dashboard");

function routePrefix(path: string) {
  return path.split("/:")[0];
}

function isRouteActive(pathname: string, path: string) {
  const prefix = routePrefix(path);

  return pathname === prefix || pathname.startsWith(`${prefix}/`);
}

function activeGroupId(pathname: string) {
  return navGroups.find((group) => group.items.some((item) => isRouteActive(pathname, item.path)))?.id;
}

export function Sidebar() {
  const location = useLocation();
  const activeGroup = useMemo(() => activeGroupId(location.pathname), [location.pathname]);
  const [openGroups, setOpenGroups] = useState<Set<string>>(() => new Set(activeGroup ? [activeGroup] : []));

  useEffect(() => {
    if (!activeGroup) {
      return;
    }

    setOpenGroups((current) => {
      if (current.has(activeGroup)) {
        return current;
      }

      return new Set([...current, activeGroup]);
    });
  }, [activeGroup]);

  return (
    <aside className="sidebar" aria-label="Основная навигация">
      <div className="sidebar-brand">ИКЦ Эксперт</div>
      {dashboardRoute ? (
        <NavLink to={dashboardRoute.path} className={({ isActive }) => `sidebar-link${isActive ? " is-active" : ""}`}>
          <dashboardRoute.icon className="sidebar-link-icon" aria-hidden />
          <span>{dashboardRoute.label}</span>
        </NavLink>
      ) : null}

      <nav className="sidebar-nav" aria-label="Разделы админки">
        {navGroups.map((group) => {
          const isOpen = openGroups.has(group.id);
          const isGroupActive = activeGroup === group.id;
          const ToggleIcon = isOpen ? ChevronDown : ChevronRight;

          return (
            <section className="sidebar-group" key={group.id}>
              <button
                className={`sidebar-group-button${isGroupActive ? " is-active" : ""}`}
                type="button"
                aria-expanded={isOpen}
                aria-controls={`sidebar-group-${group.id}`}
                onClick={() =>
                  setOpenGroups((current) => {
                    const next = new Set(current);
                    if (next.has(group.id)) {
                      next.delete(group.id);
                    } else {
                      next.add(group.id);
                    }
                    return next;
                  })
                }
              >
                <span>{group.label}</span>
                <ToggleIcon className="sidebar-group-icon" aria-hidden />
              </button>
              <div className="sidebar-subnav" id={`sidebar-group-${group.id}`} hidden={!isOpen}>
                {group.items.map((item) => (
                  <NavLink
                    key={item.id}
                    to={item.path}
                    className={() => `sidebar-sublink${isRouteActive(location.pathname, item.path) ? " is-active" : ""}`}
                  >
                    <item.icon className="sidebar-link-icon" aria-hidden />
                    <span>{item.label}</span>
                  </NavLink>
                ))}
              </div>
            </section>
          );
        })}
      </nav>
    </aside>
  );
}

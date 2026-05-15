import { useLocation, useNavigate } from "react-router";
import {
  Search,
  BookmarkCheck,
  Settings,
  LogOut,
  Sun,
  Moon,
  HelpCircle,
} from "lucide-react";
import { useThemeStore, toggleTheme } from "../stores/ui";
import { useAuthStore, clearToken } from "../stores/auth";
import {
  useOverlayStore,
  setPaletteOpen,
  setHelpOverlayOpen,
} from "../utils/keyboard";
import styles from "./Sidebar.module.css";

const NAV_ITEMS = [
  { path: "/", icon: Search, label: "Search" },
  { path: "/queries", icon: BookmarkCheck, label: "Saved Queries" },
  { path: "/settings", icon: Settings, label: "Settings" },
] as const;

function isActive(url: string, path: string): boolean {
  // Exact match for leaf routes
  if (NAV_ITEMS.some((item) => item.path === path)) {
    return url === path;
  }
  // Prefix match for routes with sub-paths
  return url === path || url.startsWith(path + "/");
}

export function Sidebar() {
  const location = useLocation();
  const navigate = useNavigate();
  const url = location.pathname;
  const theme = useThemeStore((s) => s.theme);
  const token = useAuthStore((s) => s.token);
  // Suppress unused variable warning — we subscribe to force re-renders on overlay changes
  useOverlayStore();

  return (
    <nav className={styles.sidebar}>
      <div className={styles.top}>
        <a
          href="/"
          className={styles.logo}
          onClick={(e) => {
            e.preventDefault();
            navigate("/");
          }}
        >
          <img
            src={`${import.meta.env.BASE_URL || "/"}lynxdb-icon.png`}
            alt="LynxDB"
            className={styles.logoIcon}
          />
          <span className={styles.logoText}>LynxDB</span>
        </a>
        {NAV_ITEMS.map(({ path, icon: Icon, label }) => (
          <a
            key={path}
            href={path}
            className={`${styles.navItem} ${isActive(url, path) ? styles.active : ""}`}
            title={label}
            onClick={(e) => {
              e.preventDefault();
              navigate(path);
            }}
          >
            <Icon size={20} />
            <span className={styles.navLabel}>{label}</span>
          </a>
        ))}
      </div>
      <div className={styles.bottom}>
        <button
          type="button"
          className={styles.navItem}
          onClick={toggleTheme}
          title={
            theme === "dark"
              ? "Switch to light mode"
              : "Switch to dark mode"
          }
        >
          {theme === "dark" ? <Sun size={20} /> : <Moon size={20} />}
          <span className={styles.navLabel}>
            {theme === "dark" ? "Light mode" : "Dark mode"}
          </span>
        </button>
        <button
          type="button"
          className={styles.navItem}
          onClick={() => {
            setPaletteOpen(false);
            setHelpOverlayOpen(true);
          }}
          title="Keyboard shortcuts (?)"
        >
          <HelpCircle size={20} />
          <span className={styles.navLabel}>Shortcuts</span>
        </button>
        {token && (
          <button
            type="button"
            className={styles.navItem}
            onClick={clearToken}
            title="Sign out"
          >
            <LogOut size={20} />
            <span className={styles.navLabel}>Sign out</span>
          </button>
        )}
      </div>
    </nav>
  );
}

import { useRef, useEffect, useState } from "react";
import { useNavigate } from "react-router";
import type { ComponentType } from "react";
import {
  Search,
  BookmarkCheck,
  Settings,
  Play,
  Repeat,
  Sun,
  Moon,
  PanelLeftClose,
  Keyboard,
  Clock,
} from "lucide-react";
import { useThemeStore, toggleTheme } from "../stores/ui";
import { useQueryHistoryStore } from "../stores/queryHistory";
import {
  SHORTCUTS,
  formatShortcut,
  useOverlayStore,
  setPaletteOpen,
  setHelpOverlayOpen,
  setPaletteQuery,
} from "../utils/keyboard";
import type { ShortcutDef } from "../utils/keyboard";
import styles from "./CommandPalette.module.css";

type PaletteItem = {
  id: string;
  label: string;
  section: "navigation" | "commands" | "recent";
  icon: ComponentType<{ size?: string | number }>;
  shortcut?: ShortcutDef;
  action: () => void;
};

const SECTION_LABELS: Record<PaletteItem["section"], string> = {
  navigation: "Navigation",
  commands: "Commands",
  recent: "Recent Queries",
};

const SECTION_ORDER: PaletteItem["section"][] = [
  "navigation",
  "commands",
  "recent",
];

function filterItems(items: PaletteItem[], q: string): PaletteItem[] {
  if (!q.trim()) return items;
  const lower = q.toLowerCase();
  return items
    .map((item) => {
      const label = item.label.toLowerCase();
      if (label.startsWith(lower)) return { item, score: 3 };
      if (label.split(/\s+/).some((w) => w.startsWith(lower)))
        return { item, score: 2 };
      if (label.includes(lower)) return { item, score: 1 };
      return null;
    })
    .filter((x): x is { item: PaletteItem; score: number } => x !== null)
    .sort((a, b) => b.score - a.score)
    .map((x) => x.item);
}

function truncate(text: string, max: number): string {
  return text.length > max ? text.slice(0, max) + "…" : text;
}

export function CommandPalette() {
  const inputRef = useRef<HTMLInputElement>(null);
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState(0);
  const navigate = useNavigate();

  const paletteOpen = useOverlayStore((s) => s.paletteOpen);
  const theme = useThemeStore((s) => s.theme);
  const queryHistory = useQueryHistoryStore((s) => s.queryHistory);

  const navigationItems: PaletteItem[] = [
    {
      id: "nav-search",
      label: "Search",
      section: "navigation",
      icon: Search,
      shortcut: SHORTCUTS.focusSearch,
      action: () => navigate("/"),
    },
    {
      id: "nav-queries",
      label: "Saved Queries",
      section: "navigation",
      icon: BookmarkCheck,
      action: () => navigate("/queries"),
    },
    {
      id: "nav-settings",
      label: "Settings",
      section: "navigation",
      icon: Settings,
      action: () => navigate("/settings"),
    },
  ];

  const commandItems: PaletteItem[] = [
    {
      id: "cmd-run",
      label: "Run query",
      section: "commands",
      icon: Play,
      shortcut: SHORTCUTS.runQuery,
      action: () => navigate("/"),
    },
    {
      id: "cmd-tail",
      label: "Toggle live tail",
      section: "commands",
      icon: Repeat,
      shortcut: SHORTCUTS.toggleTail,
      action: () => navigate("/"),
    },
    {
      id: "cmd-theme",
      label: `Toggle theme (${theme === "light" ? "dark" : "light"})`,
      section: "commands",
      icon: theme === "light" ? Moon : Sun,
      action: () => toggleTheme(),
    },
    {
      id: "cmd-sidebar",
      label: "Toggle sidebar",
      section: "commands",
      icon: PanelLeftClose,
      shortcut: SHORTCUTS.toggleSidebar,
      action: () => navigate("/"),
    },
    {
      id: "cmd-help",
      label: "Keyboard shortcuts",
      section: "commands",
      icon: Keyboard,
      shortcut: SHORTCUTS.openHelp,
      action: () => {
        setPaletteOpen(false);
        setHelpOverlayOpen(true);
      },
    },
  ];

  const recentItems: PaletteItem[] = queryHistory
    .slice(0, 10)
    .map((q: string, i: number) => ({
      id: `recent-${i}`,
      label: truncate(q, 60),
      section: "recent" as const,
      icon: Clock,
      action: () => {
        setPaletteQuery(q);
        navigate("/");
      },
    }));

  const allItems = [...navigationItems, ...commandItems, ...recentItems];
  const filtered = filterItems(allItems, search);

  // Reset state on open
  useEffect(() => {
    if (paletteOpen) {
      setSearch("");
      setSelected(0);
      // Auto-focus the input after rendering
      requestAnimationFrame(() => {
        inputRef.current?.focus();
      });
    }
  }, [paletteOpen]);

  // Clamp selected index when filtered list changes
  useEffect(() => {
    if (selected >= filtered.length) {
      setSelected(Math.max(0, filtered.length - 1));
    }
  }, [filtered.length, selected]);

  if (!paletteOpen) return null;

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelected((prev) => (prev + 1) % filtered.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelected((prev) => (prev - 1 + filtered.length) % filtered.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (filtered[selected]) {
        filtered[selected].action();
        setPaletteOpen(false);
      }
    } else if (e.key === "Escape") {
      e.preventDefault();
      setPaletteOpen(false);
    }
  };

  const handleBackdropClick = () => {
    setPaletteOpen(false);
  };

  // Group filtered items by section (preserving section order)
  const grouped = SECTION_ORDER.map((section) => ({
    section,
    label: SECTION_LABELS[section],
    items: filtered.filter((item) => item.section === section),
  })).filter((g) => g.items.length > 0);

  // Compute flat index offset for each group
  let flatIndex = 0;

  return (
    <div className={styles.backdrop} onClick={handleBackdropClick}>
      <div
        className={styles.palette}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        <input
          ref={inputRef}
          type="text"
          className={styles.searchInput}
          placeholder="Type a command..."
          value={search}
          onInput={(e) => {
            setSearch((e.target as HTMLInputElement).value);
            setSelected(0);
          }}
        />
        <div className={styles.results}>
          {filtered.length === 0 && <div className={styles.empty}>No matches</div>}
          {grouped.map((group) => {
            const groupStartIndex = flatIndex;
            const groupItems = group.items.map((item, i) => {
              const itemIndex = groupStartIndex + i;
              return (
                <div
                  key={item.id}
                  className={`${styles.item}${itemIndex === selected ? ` ${styles.selected}` : ""}`}
                  onClick={() => {
                    item.action();
                    setPaletteOpen(false);
                  }}
                  onMouseEnter={() => setSelected(itemIndex)}
                >
                  <item.icon size={16} />
                  <span className={styles.itemLabel}>{item.label}</span>
                  {item.shortcut && (
                    <span className={styles.itemShortcut}>
                      {formatShortcut(item.shortcut)}
                    </span>
                  )}
                </div>
              );
            });
            flatIndex += group.items.length;
            return (
              <div key={group.section}>
                <div className={styles.sectionHeader}>{group.label}</div>
                {groupItems}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

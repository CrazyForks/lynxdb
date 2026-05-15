import {
  SHORTCUTS,
  formatShortcut,
  useOverlayStore,
  setHelpOverlayOpen,
} from "../utils/keyboard";
import type { ShortcutDef } from "../utils/keyboard";
import styles from "./HelpOverlay.module.css";

type ShortcutRow = {
  def: ShortcutDef;
  label: string;
};

type ShortcutGroup = {
  title: string;
  items: ShortcutRow[];
};

const GROUPS: ShortcutGroup[] = [
  {
    title: "General",
    items: [
      { def: SHORTCUTS.openPalette, label: "Command palette" },
      { def: SHORTCUTS.openHelp, label: "Keyboard shortcuts" },
    ],
  },
  {
    title: "Query",
    items: [
      { def: SHORTCUTS.runQuery, label: "Run query" },
      { def: SHORTCUTS.focusEditor, label: "Focus editor" },
      { def: SHORTCUTS.focusSearch, label: "Focus editor (alt)" },
      { def: SHORTCUTS.historyUp, label: "Previous query" },
      { def: SHORTCUTS.historyDown, label: "Next query" },
    ],
  },
  {
    title: "Navigation",
    items: [
      { def: SHORTCUTS.toggleSidebar, label: "Toggle field sidebar" },
      { def: SHORTCUTS.toggleTail, label: "Toggle live tail" },
    ],
  },
  {
    title: "Panels",
    items: [{ def: SHORTCUTS.closePanel, label: "Close topmost panel" }],
  },
];

export function HelpOverlay() {
  const helpOverlayOpen = useOverlayStore((s) => s.helpOverlayOpen);

  if (!helpOverlayOpen) return null;

  const handleBackdropClick = () => {
    setHelpOverlayOpen(false);
  };

  return (
    <div className={styles.backdrop} onClick={handleBackdropClick}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.title}>Keyboard Shortcuts</div>
        <div className={styles.grid}>
          {GROUPS.map((group) => (
            <div key={group.title} className={styles.group}>
              <div className={styles.groupTitle}>{group.title}</div>
              {group.items.map((item) => (
                <div key={item.label} className={styles.row}>
                  <span className={styles.label}>{item.label}</span>
                  <kbd className={styles.kbd}>{formatShortcut(item.def)}</kbd>
                </div>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

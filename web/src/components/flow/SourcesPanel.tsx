import { useState, useCallback } from "react";
import type { IndexInfo, ViewSummary } from "../../api/client";
import styles from "./flow.module.css";

interface SourcesPanelProps {
  indexes: IndexInfo[];
  views: ViewSummary[];
  onSelectSource?: (name: string) => void;
}

export function SourcesPanel({
  indexes,
  views,
  onSelectSource,
}: SourcesPanelProps) {
  const [expanded, setExpanded] = useState(true);

  const handleToggle = useCallback(() => {
    setExpanded((prev) => !prev);
  }, []);

  const hasContent = indexes.length > 0 || views.length > 0;

  if (!hasContent) {
    return (
      <div className={styles.sourcesPanel}>
        <div className={styles.sectionHeader}>
          <span className={styles.sectionTitle}>Sources</span>
        </div>
        <div className={styles.sourcesEmpty}>No sources available</div>
      </div>
    );
  }

  return (
    <div className={styles.sourcesPanel}>
      <button
        type="button"
        className={styles.sectionHeader}
        onClick={handleToggle}
        aria-expanded={expanded}
      >
        <span
          className={`${styles.sectionChevron} ${expanded ? styles.sectionChevronExpanded : ""}`}
          aria-hidden="true"
        >
          &#9656;
        </span>
        <span className={styles.sectionTitle}>Sources</span>
        <span className={styles.sectionCount}>{indexes.length + views.length}</span>
      </button>

      {expanded && (
        <div className={styles.sourceList}>
          {indexes.map((idx) => (
            <button
              key={idx.name}
              type="button"
              className={styles.sourceItem}
              onClick={() => onSelectSource?.(idx.name)}
              title={`Query index: ${idx.name}`}
            >
              <span className={styles.sourceIcon} aria-hidden="true">
                &#9632;
              </span>
              <span className={styles.sourceName}>{idx.name}</span>
            </button>
          ))}
          {views.map((view) => (
            <button
              key={view.name}
              type="button"
              className={styles.sourceItem}
              onClick={() => onSelectSource?.(view.name)}
              title={`Query view: ${view.name} (${view.status})`}
            >
              <span className={styles.sourceIconView} aria-hidden="true">
                &#9670;
              </span>
              <span className={styles.sourceName}>{view.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

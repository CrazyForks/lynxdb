import { useState, useCallback } from "react";
import type { JSX } from "react";
import styles from "./EventDetail.module.css";

export interface EventDetailInlineProps {
  event: Record<string, unknown>;
  onFilter?: (field: string, value: string, exclude: boolean) => void;
}

type TabId = "fields" | "json";

// Collapsible JSON tree

interface JsonNodeProps {
  data: unknown;
  depth: number;
  keyName?: string;
}

function JsonNode({ data, depth, keyName }: JsonNodeProps): JSX.Element {
  const [expanded, setExpanded] = useState(depth < 2);

  const toggle = useCallback(() => setExpanded((v) => !v), []);

  // --- Leaf values ---
  if (data === null) {
    return (
      <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
        {keyName != null && (
          <>
            <span className={styles.jsonKey}>"{keyName}"</span>
            <span className={styles.jsonPunct}>: </span>
          </>
        )}
        <span className={styles.jsonNull}>null</span>
      </div>
    );
  }

  if (typeof data === "string") {
    return (
      <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
        {keyName != null && (
          <>
            <span className={styles.jsonKey}>"{keyName}"</span>
            <span className={styles.jsonPunct}>: </span>
          </>
        )}
        <span className={styles.jsonString}>"{data}"</span>
      </div>
    );
  }

  if (typeof data === "number") {
    return (
      <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
        {keyName != null && (
          <>
            <span className={styles.jsonKey}>"{keyName}"</span>
            <span className={styles.jsonPunct}>: </span>
          </>
        )}
        <span className={styles.jsonNumber}>{String(data)}</span>
      </div>
    );
  }

  if (typeof data === "boolean") {
    return (
      <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
        {keyName != null && (
          <>
            <span className={styles.jsonKey}>"{keyName}"</span>
            <span className={styles.jsonPunct}>: </span>
          </>
        )}
        <span className={styles.jsonBool}>{String(data)}</span>
      </div>
    );
  }

  // --- Arrays ---
  if (Array.isArray(data)) {
    const count = data.length;
    if (count === 0) {
      return (
        <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
          {keyName != null && (
            <>
              <span className={styles.jsonKey}>"{keyName}"</span>
              <span className={styles.jsonPunct}>: </span>
            </>
          )}
          <span className={styles.jsonPunct}>[]</span>
        </div>
      );
    }

    return (
      <div>
        <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
          <span className={styles.jsonToggle} onClick={toggle}>
            {expanded ? "\u25BC" : "\u25B6"}
          </span>
          {keyName != null && (
            <>
              <span className={styles.jsonKey}>"{keyName}"</span>
              <span className={styles.jsonPunct}>: </span>
            </>
          )}
          {!expanded && (
            <span className={styles.jsonCollapsed}>
              {"["} ...{count} {count === 1 ? "item" : "items"} {"]"}
            </span>
          )}
          {expanded && <span className={styles.jsonPunct}>{"["}</span>}
        </div>
        {expanded && (
          <div className={styles.jsonChildren}>
            {data.map((item, i) => (
              <JsonNode key={i} data={item} depth={depth + 1} />
            ))}
          </div>
        )}
        {expanded && (
          <div
            className={styles.jsonNode}
            style={{ paddingLeft: `${depth * 16}px` }}
          >
            <span className={styles.jsonPunct}>{"]"}</span>
          </div>
        )}
      </div>
    );
  }

  // --- Objects ---
  if (typeof data === "object") {
    const entries = Object.entries(data as Record<string, unknown>);
    const count = entries.length;
    if (count === 0) {
      return (
        <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
          {keyName != null && (
            <>
              <span className={styles.jsonKey}>"{keyName}"</span>
              <span className={styles.jsonPunct}>: </span>
            </>
          )}
          <span className={styles.jsonPunct}>{"{}"}</span>
        </div>
      );
    }

    return (
      <div>
        <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
          <span className={styles.jsonToggle} onClick={toggle}>
            {expanded ? "\u25BC" : "\u25B6"}
          </span>
          {keyName != null && (
            <>
              <span className={styles.jsonKey}>"{keyName}"</span>
              <span className={styles.jsonPunct}>: </span>
            </>
          )}
          {!expanded && (
            <span className={styles.jsonCollapsed}>
              {"{"} ...{count} {count === 1 ? "key" : "keys"} {"}"}
            </span>
          )}
          {expanded && <span className={styles.jsonPunct}>{"{"}</span>}
        </div>
        {expanded && (
          <div className={styles.jsonChildren}>
            {entries.map(([key, val]) => (
              <JsonNode key={key} data={val} depth={depth + 1} keyName={key} />
            ))}
          </div>
        )}
        {expanded && (
          <div
            className={styles.jsonNode}
            style={{ paddingLeft: `${depth * 16}px` }}
          >
            <span className={styles.jsonPunct}>{"}"}</span>
          </div>
        )}
      </div>
    );
  }

  // Fallback
  return (
    <div className={styles.jsonNode} style={{ paddingLeft: `${depth * 16}px` }}>
      {keyName != null && (
        <>
          <span className={styles.jsonKey}>"{keyName}"</span>
          <span className={styles.jsonPunct}>: </span>
        </>
      )}
      <span>{String(data)}</span>
    </div>
  );
}

// Inline event detail accordion (replaces the old slide-out panel)

export function EventDetailInline({ event, onFilter }: EventDetailInlineProps) {
  const [tab, setTab] = useState<TabId>("fields");

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(JSON.stringify(event, null, 2)).catch(() => {
      // clipboard write can fail in non-HTTPS contexts; silently ignore
    });
  }, [event]);

  const entries = Object.entries(event);

  return (
    <div className={styles.accordion}>
      <div className={styles.toolbar}>
        <button
          type="button"
          className={`${styles.tab} ${tab === "fields" ? styles.tabActive : ""}`}
          onClick={() => setTab("fields")}
        >
          Fields
        </button>
        <button
          type="button"
          className={`${styles.tab} ${tab === "json" ? styles.tabActive : ""}`}
          onClick={() => setTab("json")}
        >
          JSON
        </button>
        <div className={styles.spacer} />
        <button type="button" className={styles.copyBtn} onClick={handleCopy}>
          Copy JSON
        </button>
      </div>
      <div className={styles.body}>
        {tab === "fields" ? (
          <div className={styles.fieldsList}>
            {entries.map(([key, value]) => (
              <div key={key} className={styles.fieldRow}>
                <span className={styles.fieldKey}>{key}</span>
                <span className={styles.fieldValue}>
                  {value == null ? "" : String(value)}
                </span>
                <span className={styles.fieldActions}>
                  <button
                    type="button"
                    className={styles.filterBtn}
                    onClick={() => onFilter?.(key, String(value ?? ""), false)}
                    title={`Filter: ${key}="${value}"`}
                    aria-label={`Include ${key} equals ${value}`}
                  >
                    +
                  </button>
                  <button
                    type="button"
                    className={styles.excludeBtn}
                    onClick={() => onFilter?.(key, String(value ?? ""), true)}
                    title={`Exclude: ${key}!="${value}"`}
                    aria-label={`Exclude ${key} equals ${value}`}
                  >
                    &minus;
                  </button>
                </span>
              </div>
            ))}
          </div>
        ) : (
          <div className={styles.jsonTree}>
            <JsonNode data={event} depth={0} />
          </div>
        )}
      </div>
    </div>
  );
}

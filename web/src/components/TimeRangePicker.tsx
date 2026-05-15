import { useRef, useEffect, useCallback, useState } from "react";
import {
  PRESETS,
  getTimeRangeLabel,
  toNowExpr,
  parseNowExpression,
} from "../utils/timeFormat";
import styles from "./TimeRangePicker.module.css";

interface TimeRangePickerProps {
  from: string;
  to: string | undefined;
  onFromChange: (value: string) => void;
  onToChange: (value: string | undefined) => void;
  onApply?: () => void;
}

export function TimeRangePicker({
  from,
  to,
  onFromChange,
  onToChange,
  onApply,
}: TimeRangePickerProps) {
  const wrapperRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [fromInput, setFromInput] = useState("");
  const [toInput, setToInput] = useState("");
  const [quickSearch, setQuickSearch] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);

  // Sync inputs when dropdown opens
  useEffect(() => {
    if (open) {
      setFromInput(toNowExpr(from));
      setToInput(toNowExpr(to));
      setQuickSearch("");
      setValidationError(null);
    }
  }, [open, from, to]);

  // Apply absolute/relative inputs from left panel
  const handleApply = useCallback(() => {
    setValidationError(null);

    const parsedFrom = parseNowExpression(fromInput);
    const parsedTo = parseNowExpression(toInput);

    if (parsedFrom === null) {
      // Try as ISO date
      const d = new Date(fromInput);
      if (isNaN(d.getTime())) {
        setValidationError("Invalid From value. Use now-3h or ISO date.");
        return;
      }
      onFromChange(d.toISOString());
    } else if (parsedFrom === undefined) {
      // "now" as from doesn't make sense, but allow it
      setValidationError(
        "From cannot be 'now'. Use a relative offset like now-1h.",
      );
      return;
    } else {
      onFromChange(parsedFrom);
    }

    if (parsedTo === null) {
      const d = new Date(toInput);
      if (isNaN(d.getTime())) {
        setValidationError("Invalid To value. Use now or now-30m or ISO date.");
        return;
      }
      onToChange(d.toISOString());
    } else if (parsedTo === undefined) {
      onToChange(undefined);
    } else {
      onToChange(parsedTo);
    }

    setOpen(false);
    onApply?.();
  }, [onFromChange, onToChange, onApply, fromInput, toInput]);

  // Click a quick-range preset
  const handlePreset = useCallback(
    (value: string) => {
      onFromChange(value);
      onToChange(undefined);
      setOpen(false);
      onApply?.();
    },
    [onFromChange, onToChange, onApply],
  );

  // Close on outside click
  useEffect(() => {
    function onPointerDown(e: PointerEvent) {
      if (
        wrapperRef.current &&
        !wrapperRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    document.addEventListener("pointerdown", onPointerDown, true);
    return () =>
      document.removeEventListener("pointerdown", onPointerDown, true);
  }, []);

  // Close on Escape
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape" && open) {
        setOpen(false);
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open]);

  // Filter presets by search
  const filteredPresets = quickSearch
    ? PRESETS.filter((p) =>
        p.label.toLowerCase().includes(quickSearch.toLowerCase()),
      )
    : PRESETS;

  // Determine which preset is active
  const activePreset =
    to === undefined || to === "now"
      ? (PRESETS.find((p) => p.value === from)?.value ?? null)
      : null;

  return (
    <div className={styles.wrapper} ref={wrapperRef}>
      <button
        type="button"
        className={styles.trigger}
        onClick={() => {
          setOpen(!open);
        }}
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        <svg
          className={styles.triggerIcon}
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <circle cx="7" cy="7" r="5.5" />
          <path d="M7 4.5V7l2 1.5" />
        </svg>
        {getTimeRangeLabel(from, to)}
      </button>

      {open && (
        <div
          className={styles.dropdown}
          role="dialog"
          aria-label="Time range picker"
        >
          {/* Left panel: absolute time range */}
          <div className={styles.leftPanel}>
            <div className={styles.panelTitle}>Absolute time range</div>

            <div className={styles.inputGroup}>
              <label className={styles.inputLabel}>From</label>
              <input
                type="text"
                className={styles.textInput}
                value={fromInput}
                placeholder="now-1h"
                onInput={(e) => {
                  setFromInput((e.target as HTMLInputElement).value);
                  setValidationError(null);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    handleApply();
                  }
                }}
              />
            </div>

            <div className={styles.inputGroup}>
              <label className={styles.inputLabel}>To</label>
              <input
                type="text"
                className={styles.textInput}
                value={toInput}
                placeholder="now"
                onInput={(e) => {
                  setToInput((e.target as HTMLInputElement).value);
                  setValidationError(null);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    handleApply();
                  }
                }}
              />
            </div>

            {validationError && (
              <div className={styles.validationError}>{validationError}</div>
            )}

            <button type="button" className={styles.applyBtn} onClick={handleApply}>
              Apply time range
            </button>
          </div>

          {/* Right panel: quick ranges */}
          <div className={styles.rightPanel}>
            <input
              type="text"
              className={styles.searchInput}
              placeholder="Search quick ranges"
              value={quickSearch}
              onInput={(e) =>
                setQuickSearch((e.target as HTMLInputElement).value)
              }
            />
            <div className={styles.presetList}>
              {filteredPresets.map((preset) => (
                <button
                  key={preset.value}
                  type="button"
                  className={`${styles.presetItem} ${activePreset === preset.value ? styles.presetItemActive : ""}`}
                  onClick={() => handlePreset(preset.value)}
                >
                  {preset.label}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

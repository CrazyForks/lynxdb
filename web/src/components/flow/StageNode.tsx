import styles from "./flow.module.css";
import type { PipelineStage } from "../../api/client";

interface StageNodeProps {
  stage: PipelineStage;
  isSelected: boolean;
  onSelect: () => void;
}

export function StageNode({ stage, isSelected, onSelect }: StageNodeProps) {
  const description = stage.description || "";

  return (
    <button
      type="button"
      className={`${styles.stageCompact} ${isSelected ? styles.stageCompactSelected : ""}`}
      onClick={onSelect}
    >
      <span className={styles.stageCompactCmd}>{stage.command}</span>
      {description && (
        <span className={styles.stageCompactDesc} title={description}>
          {description}
        </span>
      )}
    </button>
  );
}

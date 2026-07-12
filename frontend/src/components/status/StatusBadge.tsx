import { motion } from "motion/react";

export type StatusTone = "neutral" | "success" | "warning" | "danger" | "info";

interface StatusBadgeProps {
  label: string;
  tone?: StatusTone;
}

export function StatusBadge({ label, tone = "neutral" }: StatusBadgeProps) {
  return (
    <motion.span
      key={`${tone}:${label}`}
      className={`status-badge status-badge-${tone}`}
      initial={{ opacity: 0.85, scale: 0.98 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ duration: 0.14, ease: "easeOut" }}
    >
      {label}
    </motion.span>
  );
}

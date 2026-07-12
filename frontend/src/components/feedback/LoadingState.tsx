interface LoadingStateProps {
  label: string;
}

export function LoadingState({ label }: LoadingStateProps) {
  return (
    <div className="loading-state" role="status" aria-label={label}>
      <span className="loading-dot" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}

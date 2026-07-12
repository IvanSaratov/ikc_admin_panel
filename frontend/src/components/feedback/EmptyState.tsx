type EmptyStateAction =
  | {
      actionLabel: string;
      onAction: () => void;
    }
  | {
      actionLabel?: never;
      onAction?: never;
    };

type EmptyStateProps = {
  title: string;
  description: string;
} & EmptyStateAction;

export function EmptyState({ title, description, actionLabel, onAction }: EmptyStateProps) {
  return (
    <section className="empty-state">
      <h2>{title}</h2>
      <p>{description}</p>
      {actionLabel ? (
        <button className="button button-primary" type="button" onClick={onAction}>
          {actionLabel}
        </button>
      ) : null}
    </section>
  );
}

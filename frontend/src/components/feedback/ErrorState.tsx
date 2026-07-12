type ErrorStateAction =
  | {
      actionLabel: string;
      onAction: () => void;
    }
  | {
      actionLabel?: never;
      onAction?: never;
    };

type ErrorStateProps = {
  title: string;
  description: string;
} & ErrorStateAction;

export function ErrorState({ title, description, actionLabel, onAction }: ErrorStateProps) {
  return (
    <section className="error-state" role="alert">
      <h2>{title}</h2>
      <p>{description}</p>
      {actionLabel ? (
        <button className="button button-secondary" type="button" onClick={onAction}>
          {actionLabel}
        </button>
      ) : null}
    </section>
  );
}

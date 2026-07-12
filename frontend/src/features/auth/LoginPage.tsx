import { FormEvent, useState } from "react";

type LoginInput = {
  login: string;
  password: string;
};

type LoginPageProps = {
  onLogin?: (input: LoginInput) => Promise<void>;
};

export function LoginPage({ onLogin }: LoginPageProps) {
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(false);
    setIsSubmitting(true);

    try {
      await onLogin?.({ login, password });
    } catch {
      setError(true);
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <main className="login-page">
      <form className="login-card" onSubmit={handleSubmit}>
        <h1>Вход</h1>
        {error ? <p role="alert">Не удалось войти</p> : null}
        <label>
          Логин
          <input
            autoComplete="username"
            name="login"
            value={login}
            onChange={(event) => setLogin(event.target.value)}
          />
        </label>
        <label>
          Пароль
          <input
            autoComplete="current-password"
            name="password"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>
        <button className="button button-primary" type="submit" disabled={isSubmitting}>
          Войти
        </button>
      </form>
    </main>
  );
}

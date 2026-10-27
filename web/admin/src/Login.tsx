import { useState } from 'react';
import { api, ApiError, jsonRequest } from './api';
import { useI18n } from './i18n';

/** Login 顯示單一管理員密碼登入；首次密碼由服務日誌提供。 */
export function Login({ service, version, onAuthenticated }: {
  service?: string;
  version?: string;
  onAuthenticated: () => void;
}) {
  const { t } = useI18n();
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function submit() {
    setBusy(true);
    setError('');
    try {
      await api('auth/login', jsonRequest('POST', { password }));
      setPassword('');
      onAuthenticated();
    } catch (reason) {
      setError(reason instanceof ApiError ? reason.message : t('login.failed'));
    } finally {
      setBusy(false);
    }
  }

  return <section className="login-panel">
    <h2>{t('login.title')}</h2>
    <p className="muted">{t('login.hint1')}</p>
    <p className="muted">{t('login.hint2')}</p>
    <form onSubmit={event => { event.preventDefault(); void submit(); }}>
      <label>{t('login.password')}<input type="password" autoFocus autoComplete="current-password" required
        value={password} onChange={event => setPassword(event.target.value)} /></label>
      {error && <p role="alert">{error}</p>}
      <button disabled={busy}>{busy ? t('login.submitting') : t('login.submit')}</button>
    </form>
    {(service || version) && <p className="muted login-meta">{service}{service && version ? t('common.dot') : ''}{version}</p>}
  </section>;
}

import { useState } from 'react';
import { api, ApiError, jsonRequest } from './api';

/** Login 顯示單一管理員密碼登入；首次密碼由服務日誌提供。 */
export function Login({ service, version, onAuthenticated }: {
  service?: string;
  version?: string;
  onAuthenticated: () => void;
}) {
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
      setError(reason instanceof ApiError ? reason.message : '登入失敗，請稍後再試');
    } finally {
      setBusy(false);
    }
  }

  return <section className="login-panel">
    <h2>管理登入</h2>
    <p className="muted">第一次啟動的管理員密碼會顯示在服務日誌；登入後可於設定頁變更。</p>
    <p className="muted">登入狀態會在使用中自動延長；長時間未使用或超過上限時需重新登入。</p>
    <form onSubmit={event => { event.preventDefault(); void submit(); }}>
      <label>管理密碼<input type="password" autoFocus autoComplete="current-password" required
        value={password} onChange={event => setPassword(event.target.value)} /></label>
      {error && <p role="alert">{error}</p>}
      <button disabled={busy}>{busy ? '登入中…' : '登入'}</button>
    </form>
    {(service || version) && <p className="muted login-meta">{service}{service && version ? ' · ' : ''}{version}</p>}
  </section>;
}

import { useEffect, useState } from 'react';
import { api, ApiError, jsonRequest } from './api';

type Config = {
  host: string; preferredPort: number; fallbackPorts: number[] | null;
  webDirectory: string; dataDirectory: string; autoScan: boolean;
  scanIntervalSeconds: number; fileStableSeconds: number; rememberLastSuccessfulPort: boolean;
};
type Overrides = Partial<{
  host: string; preferredPort: number; fallbackPorts: number[];
  autoScan: boolean; scanIntervalSeconds: number; fileStableSeconds: number; rememberLastSuccessfulPort: boolean;
}>;
type Network = {
  stateFile: string; rememberEnabled: boolean; previousPort: number; activePort: number; saved: boolean; warning?: string;
};
type ConfigView = {
  source: string; overridePath: string; base: Config; effective: Config;
  overrides: Overrides; editable: string[]; restartRequired: boolean; network?: Network;
};
type FormState = {
  host: string; preferredPort: string; fallbackPorts: string;
  autoScan: boolean; scanIntervalSeconds: string; fileStableSeconds: string;
  rememberLastSuccessfulPort: boolean;
};

function formFrom(config: Config): FormState {
  return {
    host: config.host,
    preferredPort: String(config.preferredPort),
    fallbackPorts: (config.fallbackPorts || []).join(', '),
    autoScan: config.autoScan,
    scanIntervalSeconds: String(config.scanIntervalSeconds),
    fileStableSeconds: String(config.fileStableSeconds),
    rememberLastSuccessfulPort: config.rememberLastSuccessfulPort,
  };
}

/** Settings 提供可編輯設定、來源診斷、密碼變更與重新啟動流程。 */
export function Settings({ service }: { service?: string }) {
  const [view, setView] = useState<ConfigView | null>(null);
  const [form, setForm] = useState<FormState | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [confirmingReset, setConfirmingReset] = useState(false);
  const [confirmingRestart, setConfirmingRestart] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  useEffect(() => {
    let active = true;
    api<ConfigView>('config')
      .then(result => { if (active) { setView(result); setForm(formFrom(result.effective)); } })
      .catch(reason => { if (active) setError(reason instanceof Error ? reason.message : String(reason)); });
    return () => { active = false; };
  }, []);

  function patch(change: Partial<FormState>) {
    setForm(current => current ? { ...current, ...change } : current);
    setNotice('');
  }

  function payload() {
    if (!form) throw new Error('設定尚未載入');
    const preferredPort = Number(form.preferredPort);
    if (!Number.isInteger(preferredPort) || preferredPort < 1 || preferredPort > 65535) {
      throw new Error('優先端口必須是 1 到 65535 的整數');
    }
    const ports = form.fallbackPorts.split(/[\s,，]+/).filter(Boolean).map(value => {
      const port = Number(value);
      if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error(`遞補端口無效：${value}`);
      return port;
    });
    const unique = [...new Set(ports)].filter(port => port !== preferredPort);
    const scanIntervalSeconds = Number(form.scanIntervalSeconds);
    const fileStableSeconds = Number(form.fileStableSeconds);
    if (!Number.isInteger(scanIntervalSeconds) || scanIntervalSeconds < 1 || scanIntervalSeconds > 86400) {
      throw new Error('掃描間隔必須是 1 到 86400 的整數');
    }
    if (!Number.isInteger(fileStableSeconds) || fileStableSeconds < 1 || fileStableSeconds > 86400) {
      throw new Error('檔案穩定時間必須是 1 到 86400 的整數');
    }
    const host = form.host.trim();
    if (!host) throw new Error('監聽介面不可為空');
    return {
      host, preferredPort, fallbackPorts: unique,
      rememberLastSuccessfulPort: unique.length > 0 && form.rememberLastSuccessfulPort,
      autoScan: form.autoScan, scanIntervalSeconds, fileStableSeconds,
    };
  }

  async function save() {
    setBusy(true); setError(''); setNotice('');
    try {
      const next = await api<ConfigView>('config', jsonRequest('PUT', payload()));
      setView(next);
      setForm(formFrom(next.effective));
      setNotice(next.restartRequired ? '設定已儲存，需重新啟動才會套用。' : '設定已儲存，與目前執行值相同。');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  }

  async function reset() {
    setBusy(true); setError(''); setNotice(''); setConfirmingReset(false);
    try {
      const next = await api<ConfigView>('config/overrides', { method: 'DELETE' });
      setView(next);
      setForm(formFrom(next.effective));
      setNotice(next.restartRequired ? '已回到基準設定，需重新啟動才會套用。' : '已回到基準設定。');
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  }

  async function restart() {
    setBusy(true); setError(''); setNotice(''); setConfirmingRestart(false); setRestarting(true);
    try {
      await api('system/restart', jsonRequest('POST', { confirm: true }));
      await new Promise(resolve => setTimeout(resolve, 1500));
      const started = Date.now();
      while (Date.now() - started < 30000) {
        try {
          const response = await fetch('/api/v1/status', { cache: 'no-store' });
          if (response.ok) { window.location.reload(); return; }
        } catch { /* 服務正在重新啟動。 */ }
        await new Promise(resolve => setTimeout(resolve, 1000));
      }
      throw new Error('服務未在 30 秒內恢復；請查看日誌或手動啟動。');
    } catch (reason) {
      setRestarting(false);
      setBusy(false);
      setError(reason instanceof ApiError ? reason.message : reason instanceof Error ? reason.message : String(reason));
    }
  }

  async function changePassword() {
    setError(''); setNotice('');
    if (newPassword.length < 12) { setError('新密碼長度至少 12 個字元'); return; }
    if (newPassword !== confirmPassword) { setError('兩次輸入的新密碼不一致'); return; }
    setBusy(true);
    try {
      await api('auth/password', jsonRequest('POST', {
        currentPassword, newPassword,
      }));
      setCurrentPassword(''); setNewPassword(''); setConfirmPassword('');
      setNotice('管理密碼已更新，其他裝置的登入狀態已失效。');
    } catch (reason) {
      setError(reason instanceof ApiError ? reason.message : '無法更新密碼');
    } finally {
      setBusy(false);
    }
  }

  if (!view || !form) {
    return <section><h2>設定</h2>{error ? <p role="alert">{error}</p> : <p role="status">正在讀取設定…</p>}</section>;
  }

  const overridden = Object.values(view.overrides).filter(value => value !== undefined && value !== null).length;
  const fallbackEmpty = form.fallbackPorts.trim() === '';

  return <div className="split-layout settings-layout">
    <section><div className="section-heading"><h2>可編輯設定</h2>{view.restartRequired && <span className="state-badge">等待重新啟動</span>}</div>
      <form className="form-stack" onSubmit={event => { event.preventDefault(); void save(); }}>
        <label>監聽介面<input value={form.host} onChange={event => patch({ host: event.target.value })} placeholder="例如 0.0.0.0 或 127.0.0.1" /></label>
        <label>優先端口<input type="number" min={1} max={65535} value={form.preferredPort} onChange={event => patch({ preferredPort: event.target.value })} /></label>
        <label>遞補端口<input value={form.fallbackPorts} onChange={event => patch({ fallbackPorts: event.target.value })} placeholder="以逗號分隔；留空表示不遞補" /></label>
        <label className="checkbox-label"><input type="checkbox" checked={fallbackEmpty ? false : form.rememberLastSuccessfulPort} disabled={fallbackEmpty}
          onChange={event => patch({ rememberLastSuccessfulPort: event.target.checked })} />下次啟動優先沿用上次成功端口</label>
        {service === 'video-node' && <>
          <label className="checkbox-label"><input type="checkbox" checked={form.autoScan} onChange={event => patch({ autoScan: event.target.checked })} />啟用自動掃描</label>
          <label>掃描間隔（秒）<input type="number" min={1} max={86400} value={form.scanIntervalSeconds} onChange={event => patch({ scanIntervalSeconds: event.target.value })} /></label>
          <label>檔案穩定時間（秒）<input type="number" min={1} max={86400} value={form.fileStableSeconds} onChange={event => patch({ fileStableSeconds: event.target.value })} /></label>
        </>}
        <div className="button-row"><button disabled={busy}>{busy ? '儲存中…' : '儲存設定'}</button>
          <button type="button" disabled={busy} onClick={() => setConfirmingReset(true)}>重設為基準</button></div>
      </form>
      {confirmingReset && <div className="confirm-panel"><p>確定移除管理頁的全部設定覆寫，回到主機 JSON 的基準值？</p>
        <div className="button-row"><button type="button" disabled={busy} onClick={() => void reset()}>確認重設</button>
          <button type="button" disabled={busy} onClick={() => setConfirmingReset(false)}>取消</button></div></div>}
    </section>
    <div className="settings-side">
      <section><h2>設定來源</h2>
        <dl>
          <div><dt>基準檔案</dt><dd>{view.source}</dd></div>
          <div><dt>覆寫檔案</dt><dd>{view.overridePath}</dd></div>
          <div><dt>資料目錄</dt><dd>{view.effective.dataDirectory}</dd></div>
          <div><dt>網頁目錄</dt><dd>{view.effective.webDirectory}</dd></div>
          <div><dt>覆寫欄位</dt><dd>{overridden ? `${overridden} 個` : '無'}</dd></div>
        </dl>
        <p className="muted">資料目錄與網頁目錄只顯示，不開放由管理頁變更。</p>
      </section>
      <section><h2>套用與重新啟動</h2>
        {restarting ? <p role="status">正在重新啟動，等待服務恢復…</p> : <>
          <p>{view.restartRequired ? '目前有已儲存的設定尚未套用。' : '目前執行值與已儲存設定相同。'}</p>
          <p className="muted">重新啟動會中斷進行中的請求；若修改了監聽介面或端口，請改用新位址開啟。</p>
          {!confirmingRestart && <button disabled={busy || !view.restartRequired} onClick={() => setConfirmingRestart(true)}>重新啟動服務</button>}
          {confirmingRestart && <div className="confirm-panel"><p>確定要重新啟動服務並套用目前已儲存的設定？</p>
            <div className="button-row"><button disabled={busy} onClick={() => void restart()}>確認重新啟動</button>
              <button disabled={busy} onClick={() => setConfirmingRestart(false)}>取消</button></div></div>}
        </>}
      </section>
      <section><h2>管理密碼</h2>
        <form className="form-stack" onSubmit={event => { event.preventDefault(); void changePassword(); }}>
          <label>目前密碼<input type="password" autoComplete="current-password" value={currentPassword} onChange={event => setCurrentPassword(event.target.value)} /></label>
          <label>新密碼<input type="password" autoComplete="new-password" value={newPassword} onChange={event => setNewPassword(event.target.value)} /></label>
          <label>確認新密碼<input type="password" autoComplete="new-password" value={confirmPassword} onChange={event => setConfirmPassword(event.target.value)} /></label>
          <button disabled={busy}>更新密碼</button>
        </form>
      </section>
      {(error || notice) && <section>{error ? <p role="alert">{error}</p> : <p className="notice">{notice}</p>}</section>}
    </div>
  </div>;
}

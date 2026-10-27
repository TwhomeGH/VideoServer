import { useEffect, useState } from 'react';
import { api, ApiError, jsonRequest } from './api';
import { useI18n } from './i18n';

type Config = {
  host: string; preferredPort: number; fallbackPorts: number[] | null;
  webDirectory: string; dataDirectory: string; autoScan: boolean;
  scanIntervalSeconds: number; scanIdleMaxSeconds: number; fileStableSeconds: number; rememberLastSuccessfulPort: boolean;
  autoBackfill: boolean; backfillIntervalSeconds: number; backfillDailyLimit: number; backfillDailyGiB: number; hashLimitMiBps: number;
};
type Overrides = Partial<{
  host: string; preferredPort: number; fallbackPorts: number[];
  autoScan: boolean; scanIntervalSeconds: number; scanIdleMaxSeconds: number; fileStableSeconds: number; rememberLastSuccessfulPort: boolean;
  autoBackfill: boolean; backfillIntervalSeconds: number; backfillDailyLimit: number; backfillDailyGiB: number; hashLimitMiBps: number;
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
  autoScan: boolean; scanIntervalSeconds: string; scanIdleMaxSeconds: string; fileStableSeconds: string;
  rememberLastSuccessfulPort: boolean;
  autoBackfill: boolean; backfillIntervalSeconds: string; backfillDailyLimit: string; backfillDailyGiB: string; hashLimitMiBps: string;
};

function formFrom(config: Config): FormState {
  return {
    host: config.host,
    preferredPort: String(config.preferredPort),
    fallbackPorts: (config.fallbackPorts || []).join(', '),
    autoScan: config.autoScan,
    scanIntervalSeconds: String(config.scanIntervalSeconds),
    fileStableSeconds: String(config.fileStableSeconds),
    scanIdleMaxSeconds: String(config.scanIdleMaxSeconds),
    rememberLastSuccessfulPort: config.rememberLastSuccessfulPort,
    autoBackfill: config.autoBackfill,
    backfillIntervalSeconds: String(config.backfillIntervalSeconds),
    backfillDailyLimit: String(config.backfillDailyLimit),
    backfillDailyGiB: String(config.backfillDailyGiB),
    hashLimitMiBps: String(config.hashLimitMiBps),
  };
}

/** Settings 提供可編輯設定、來源診斷、密碼變更與重新啟動流程。 */
export function Settings({ service }: { service?: string }) {
  const { t } = useI18n();
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
    if (!form) throw new Error(t('settings.errNotLoaded'));
    const preferredPort = Number(form.preferredPort);
    if (!Number.isInteger(preferredPort) || preferredPort < 1 || preferredPort > 65535) {
      throw new Error(t('settings.errPreferredPort'));
    }
    const ports = form.fallbackPorts.split(/[\s,，]+/).filter(Boolean).map(value => {
      const port = Number(value);
      if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error(t('settings.errFallbackPort', { value }));
      return port;
    });
    const unique = [...new Set(ports)].filter(port => port !== preferredPort);
    const scanIntervalSeconds = Number(form.scanIntervalSeconds);
    const fileStableSeconds = Number(form.fileStableSeconds);
    if (!Number.isInteger(scanIntervalSeconds) || scanIntervalSeconds < 1 || scanIntervalSeconds > 86400) {
      throw new Error(t('settings.errScanInterval'));
    }
    if (!Number.isInteger(fileStableSeconds) || fileStableSeconds < 1 || fileStableSeconds > 86400) {
      throw new Error(t('settings.errFileStable'));
    }
    const scanIdleMaxSeconds = Number(form.scanIdleMaxSeconds);
    if (!Number.isInteger(scanIdleMaxSeconds) || scanIdleMaxSeconds < 10 || scanIdleMaxSeconds > 86400) {
      throw new Error(t('settings.errScanIdleMax'));
    }
    const backfillIntervalSeconds = Number(form.backfillIntervalSeconds);
    if (!Number.isInteger(backfillIntervalSeconds) || backfillIntervalSeconds < 10 || backfillIntervalSeconds > 86400) {
      throw new Error(t('settings.errBackfillInterval'));
    }
    const hashLimitMiBps = Number(form.hashLimitMiBps);
    if (!Number.isInteger(hashLimitMiBps) || hashLimitMiBps < 0 || hashLimitMiBps > 102400) {
      throw new Error(t('settings.errHashLimit'));
    }
    const backfillDailyLimit = Number(form.backfillDailyLimit);
    if (!Number.isInteger(backfillDailyLimit) || backfillDailyLimit < 0 || backfillDailyLimit > 100000) {
      throw new Error(t('settings.errBackfillDailyLimit'));
    }
    const backfillDailyGiB = Number(form.backfillDailyGiB);
    if (!Number.isInteger(backfillDailyGiB) || backfillDailyGiB < 0 || backfillDailyGiB > 102400) {
      throw new Error(t('settings.errBackfillDailyGiB'));
    }
    const host = form.host.trim();
    if (!host) throw new Error(t('settings.errHost'));
    return {
      host, preferredPort, fallbackPorts: unique,
      rememberLastSuccessfulPort: unique.length > 0 && form.rememberLastSuccessfulPort,
      autoScan: form.autoScan, scanIntervalSeconds, scanIdleMaxSeconds, fileStableSeconds,
      autoBackfill: form.autoBackfill, backfillIntervalSeconds, backfillDailyLimit, backfillDailyGiB, hashLimitMiBps,
    };
  }

  async function save() {
    setBusy(true); setError(''); setNotice('');
    try {
      const next = await api<ConfigView>('config', jsonRequest('PUT', payload()));
      setView(next);
      setForm(formFrom(next.effective));
      setNotice(next.restartRequired ? t('settings.noticeSavedRestart') : t('settings.noticeSavedSame'));
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
      setNotice(next.restartRequired ? t('settings.noticeResetRestart') : t('settings.noticeReset'));
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
      throw new Error(t('settings.errRestartTimeout'));
    } catch (reason) {
      setRestarting(false);
      setBusy(false);
      setError(reason instanceof ApiError ? reason.message : reason instanceof Error ? reason.message : String(reason));
    }
  }

  async function changePassword() {
    setError(''); setNotice('');
    if (newPassword.length < 12) { setError(t('settings.errNewPasswordLength')); return; }
    if (newPassword !== confirmPassword) { setError(t('settings.errPasswordMismatch')); return; }
    setBusy(true);
    try {
      await api('auth/password', jsonRequest('POST', {
        currentPassword, newPassword,
      }));
      setCurrentPassword(''); setNewPassword(''); setConfirmPassword('');
      setNotice(t('settings.noticePassword'));
    } catch (reason) {
      setError(reason instanceof ApiError ? reason.message : t('settings.errPasswordUpdate'));
    } finally {
      setBusy(false);
    }
  }

  if (!view || !form) {
    return <section><h2>{t('settings.title')}</h2>{error ? <p role="alert">{error}</p> : <p role="status">{t('settings.reading')}</p>}</section>;
  }

  const overridden = Object.values(view.overrides).filter(value => value !== undefined && value !== null).length;
  const fallbackEmpty = form.fallbackPorts.trim() === '';

  return <div className="split-layout settings-layout">
    <section><div className="section-heading"><h2>{t('settings.editableTitle')}</h2>{view.restartRequired && <span className="state-badge">{t('settings.waitingRestart')}</span>}</div>
      <form className="form-stack" onSubmit={event => { event.preventDefault(); void save(); }}>
        <label>{t('settings.host')}<input value={form.host} onChange={event => patch({ host: event.target.value })} placeholder={t('settings.hostPlaceholder')} /></label>
        <label>{t('settings.preferredPort')}<input type="number" min={1} max={65535} value={form.preferredPort} onChange={event => patch({ preferredPort: event.target.value })} /></label>
        <label>{t('settings.fallbackPorts')}<input value={form.fallbackPorts} onChange={event => patch({ fallbackPorts: event.target.value })} placeholder={t('settings.fallbackPlaceholder')} /></label>
        <label className="checkbox-label"><input type="checkbox" checked={fallbackEmpty ? false : form.rememberLastSuccessfulPort} disabled={fallbackEmpty}
          onChange={event => patch({ rememberLastSuccessfulPort: event.target.checked })} />{t('settings.rememberPort')}</label>
        {fallbackEmpty && <p className="muted">{t('settings.rememberHint')}</p>}
        {service === 'video-node' && <>
          <fieldset className="settings-group"><legend>{t('settings.scanGroup')}</legend>
            <label className="checkbox-label"><input type="checkbox" checked={form.autoScan} onChange={event => patch({ autoScan: event.target.checked })} />{t('settings.autoScan')}</label>
            <label>{t('settings.scanInterval')}<input type="number" min={1} max={86400} value={form.scanIntervalSeconds} onChange={event => patch({ scanIntervalSeconds: event.target.value })} /></label>
            <label>{t('settings.scanIdleMax')}<input type="number" min={10} max={86400} value={form.scanIdleMaxSeconds} onChange={event => patch({ scanIdleMaxSeconds: event.target.value })} /></label>
            <p className="muted">{t('settings.scanIdleMaxHint')}</p>
            <label>{t('settings.fileStable')}<input type="number" min={1} max={86400} value={form.fileStableSeconds} onChange={event => patch({ fileStableSeconds: event.target.value })} /></label>
          </fieldset>
          <fieldset className="settings-group"><legend>{t('settings.backfillGroup')}</legend>
            <label className="checkbox-label"><input type="checkbox" checked={form.autoBackfill} onChange={event => patch({ autoBackfill: event.target.checked })} />{t('settings.autoBackfill')}</label>
            <p className="muted">{t('settings.autoBackfillHint')}</p>
            <label>{t('settings.backfillInterval')}<input type="number" min={10} max={86400} value={form.backfillIntervalSeconds} onChange={event => patch({ backfillIntervalSeconds: event.target.value })} /></label>
            <label>{t('settings.backfillDailyLimit')}<input type="number" min={0} max={100000} value={form.backfillDailyLimit} onChange={event => patch({ backfillDailyLimit: event.target.value })} /></label>
            <label>{t('settings.backfillDailyGiB')}<input type="number" min={0} max={102400} value={form.backfillDailyGiB} onChange={event => patch({ backfillDailyGiB: event.target.value })} /></label>
            <label>{t('settings.hashLimit')}<input type="number" min={0} max={102400} value={form.hashLimitMiBps} onChange={event => patch({ hashLimitMiBps: event.target.value })} /></label>
          </fieldset>
        </>}
        <div className="button-row"><button disabled={busy}>{busy ? t('settings.saving') : t('settings.save')}</button></div>
      </form>
      <div className="settings-danger">
        <h3>{t('settings.overrideResetTitle')}</h3>
        <p className="muted">{t('settings.overrideResetHint', { base: view.source, override: view.overridePath })}</p>
        {!confirmingReset && <button type="button" disabled={busy} onClick={() => setConfirmingReset(true)}>{t('settings.reset')}</button>}
        {confirmingReset && <div className="confirm-panel"><p>{t('settings.resetConfirm', { base: view.source, override: view.overridePath })}</p>
          <div className="button-row"><button type="button" disabled={busy} onClick={() => void reset()}>{t('settings.resetConfirmButton')}</button>
            <button type="button" disabled={busy} onClick={() => setConfirmingReset(false)}>{t('common.cancel')}</button></div></div>}
      </div>
    </section>
    <div className="settings-side">
      <section><h2>{t('settings.sourceTitle')}</h2>
        <dl>
          <div><dt>{t('settings.baseFile')}</dt><dd>{view.source}</dd></div>
          <div><dt>{t('settings.overrideFile')}</dt><dd>{view.overridePath}</dd></div>
          <div><dt>{t('settings.dataDir')}</dt><dd>{view.effective.dataDirectory}</dd></div>
          <div><dt>{t('settings.webDir')}</dt><dd>{view.effective.webDirectory}</dd></div>
          <div><dt>{t('settings.overriddenFields')}</dt><dd>{overridden ? t('common.overriddenCount', { count: overridden }) : t('settings.none')}</dd></div>
        </dl>
        <p className="muted">{t('settings.dirsReadOnly')}</p>
      </section>
      <section><h2>{t('settings.applyTitle')}</h2>
        {restarting ? <p role="status">{t('settings.restarting')}</p> : <>
          <p>{view.restartRequired ? t('settings.restartRequired') : t('settings.restartNotRequired')}</p>
          <p className="muted">{t('settings.restartWarning')}</p>
          {!confirmingRestart && <button disabled={busy || !view.restartRequired} onClick={() => setConfirmingRestart(true)}>{t('settings.restart')}</button>}
          {confirmingRestart && <div className="confirm-panel"><p>{t('settings.restartConfirm')}</p>
            <div className="button-row"><button disabled={busy} onClick={() => void restart()}>{t('settings.restartConfirmButton')}</button>
              <button disabled={busy} onClick={() => setConfirmingRestart(false)}>{t('common.cancel')}</button></div></div>}
        </>}
      </section>
      <section><h2>{t('settings.passwordTitle')}</h2>
        <form className="form-stack" onSubmit={event => { event.preventDefault(); void changePassword(); }}>
          <label>{t('settings.currentPassword')}<input type="password" autoComplete="current-password" value={currentPassword} onChange={event => setCurrentPassword(event.target.value)} /></label>
          <label>{t('settings.newPassword')}<input type="password" autoComplete="new-password" value={newPassword} onChange={event => setNewPassword(event.target.value)} /></label>
          <label>{t('settings.confirmPassword')}<input type="password" autoComplete="new-password" value={confirmPassword} onChange={event => setConfirmPassword(event.target.value)} /></label>
          <button disabled={busy}>{t('settings.updatePassword')}</button>
        </form>
      </section>
      {(error || notice) && <section>{error ? <p role="alert">{error}</p> : <p className="notice">{notice}</p>}</section>}
    </div>
  </div>;
}

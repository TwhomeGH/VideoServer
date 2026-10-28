import { useEffect, useRef, useState } from 'react';
import { api, ApiError, jsonRequest } from './api';
import { DurationField } from './DurationField';
import { useI18n } from './i18n';
import { useLiveVersion } from './live';

type Config = {
  host: string; preferredPort: number; fallbackPorts: number[] | null;
  webDirectory: string; dataDirectory: string; autoScan: boolean;
  scanIntervalSeconds: number; scanWarmSeconds: number; scanWarmSlowSeconds: number; scanColdSeconds: number; scanIdleMaxSeconds: number; fileStableSeconds: number; rememberLastSuccessfulPort: boolean;
  autoBackfill: boolean; backfillIntervalSeconds: number; backfillDailyLimit: number; backfillDailyGiB: number; hashLimitMiBps: number;
  autoCover: boolean; coverIntervalSeconds: number;
};
type Overrides = Partial<{
  host: string; preferredPort: number; fallbackPorts: number[];
  autoScan: boolean; scanIntervalSeconds: number; scanWarmSeconds: number; scanWarmSlowSeconds: number; scanColdSeconds: number; scanIdleMaxSeconds: number; fileStableSeconds: number; rememberLastSuccessfulPort: boolean;
  autoBackfill: boolean; backfillIntervalSeconds: number; backfillDailyLimit: number; backfillDailyGiB: number; hashLimitMiBps: number;
  autoCover: boolean; coverIntervalSeconds: number;
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
  autoScan: boolean; scanIntervalSeconds: string; scanWarmSeconds: string; scanWarmSlowSeconds: string; scanColdSeconds: string; scanIdleMaxSeconds: string; fileStableSeconds: string;
  rememberLastSuccessfulPort: boolean;
  autoBackfill: boolean; backfillIntervalSeconds: string; backfillDailyLimit: string; backfillDailyGiB: string; hashLimitMiBps: string;
  autoCover: boolean; coverIntervalSeconds: string;
};

function formFrom(config: Config): FormState {
  return {
    host: config.host,
    preferredPort: String(config.preferredPort),
    fallbackPorts: (config.fallbackPorts || []).join(', '),
    autoScan: config.autoScan,
    scanIntervalSeconds: String(config.scanIntervalSeconds),
    scanWarmSeconds: String(config.scanWarmSeconds),
    scanWarmSlowSeconds: String(config.scanWarmSlowSeconds),
    scanColdSeconds: String(config.scanColdSeconds),
    fileStableSeconds: String(config.fileStableSeconds),
    scanIdleMaxSeconds: String(config.scanIdleMaxSeconds),
    rememberLastSuccessfulPort: config.rememberLastSuccessfulPort,
    autoBackfill: config.autoBackfill,
    backfillIntervalSeconds: String(config.backfillIntervalSeconds),
    backfillDailyLimit: String(config.backfillDailyLimit),
    backfillDailyGiB: String(config.backfillDailyGiB),
    hashLimitMiBps: String(config.hashLimitMiBps),
    autoCover: config.autoCover,
    coverIntervalSeconds: String(config.coverIntervalSeconds),
  };
}

/** Settings 提供可編輯設定、來源診斷、密碼變更與重新啟動流程。 */
export function Settings({ service }: { service?: string }) {
  const { t } = useI18n();
  const liveConfig = useLiveVersion('config');
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
  const [dirty, setDirty] = useState(false);
  const [liveChanged, setLiveChanged] = useState(false);
  const loadVersion = useRef(0);

  async function loadConfig() {
    const version = ++loadVersion.current;
    try {
      const result = await api<ConfigView>('config');
      if (version !== loadVersion.current) return;
      setView(result);
      setForm(formFrom(result.effective));
      setDirty(false);
      setLiveChanged(false);
      setError('');
    } catch (reason) {
      if (version === loadVersion.current) setError(reason instanceof Error ? reason.message : String(reason));
    }
  }

  useEffect(() => { void loadConfig(); }, []);

  const lastLiveConfig = useRef(liveConfig);
  useEffect(() => {
    if (lastLiveConfig.current === liveConfig) return;
    lastLiveConfig.current = liveConfig;
    if (dirty) setLiveChanged(true);
    else void loadConfig();
  }, [liveConfig, dirty]);

  function patch(change: Partial<FormState>) {
    setForm(current => current ? { ...current, ...change } : current);
    setDirty(true);
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
    const scanWarmSeconds = Number(form.scanWarmSeconds);
    const scanWarmSlowSeconds = Number(form.scanWarmSlowSeconds);
    const scanColdSeconds = Number(form.scanColdSeconds);
    if (![scanWarmSeconds, scanWarmSlowSeconds, scanColdSeconds].every(value => Number.isInteger(value) && value >= 1 && value <= 86400)) {
      throw new Error(t('settings.errScanIntervals'));
    }
    if (scanWarmSeconds < scanIntervalSeconds || scanWarmSlowSeconds < scanWarmSeconds || scanColdSeconds < scanWarmSlowSeconds) {
      throw new Error(t('settings.errScanOrder'));
    }
    const scanIdleMaxSeconds = Number(form.scanIdleMaxSeconds);
    if (!Number.isInteger(scanIdleMaxSeconds) || scanIdleMaxSeconds < 1800 || scanIdleMaxSeconds > 86400) {
      throw new Error(t('settings.errScanIdleMax'));
    }
    if (scanIdleMaxSeconds < scanColdSeconds) {
      throw new Error(t('settings.errScanOrder'));
    }
    const backfillIntervalSeconds = Number(form.backfillIntervalSeconds);
    if (!Number.isInteger(backfillIntervalSeconds) || backfillIntervalSeconds < 10 || backfillIntervalSeconds > 86400) {
      throw new Error(t('settings.errBackfillInterval'));
    }
    const coverIntervalSeconds = Number(form.coverIntervalSeconds);
    if (!Number.isInteger(coverIntervalSeconds) || coverIntervalSeconds < 10 || coverIntervalSeconds > 86400) {
      throw new Error(t('settings.errCoverInterval'));
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
      autoScan: form.autoScan, scanIntervalSeconds, scanWarmSeconds, scanWarmSlowSeconds, scanColdSeconds, scanIdleMaxSeconds, fileStableSeconds,
      autoBackfill: form.autoBackfill, backfillIntervalSeconds, backfillDailyLimit, backfillDailyGiB, hashLimitMiBps,
      autoCover: form.autoCover, coverIntervalSeconds,
    };
  }

  async function save() {
    setBusy(true); setError(''); setNotice('');
    try {
      const next = await api<ConfigView>('config', jsonRequest('PUT', payload()));
      setView(next);
      setForm(formFrom(next.effective));
      setDirty(false);
      setLiveChanged(false);
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
      setDirty(false);
      setLiveChanged(false);
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

  return <>
    {liveChanged && <section><p role="status">{t('settings.liveChanged')}</p><button type="button" disabled={busy} onClick={() => { setLiveChanged(false); void loadConfig(); }}>{t('settings.liveReload')}</button></section>}
  <div className="split-layout settings-layout">
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
            <DurationField label={t('settings.scanInterval')} seconds={Number(form.scanIntervalSeconds)} min={1} max={86400}
              onChange={seconds => patch({ scanIntervalSeconds: String(seconds) })} />
            <DurationField label={t('settings.scanWarm')} seconds={Number(form.scanWarmSeconds)} min={1} max={86400}
              onChange={seconds => patch({ scanWarmSeconds: String(seconds) })} />
            <DurationField label={t('settings.scanWarmSlow')} seconds={Number(form.scanWarmSlowSeconds)} min={1} max={86400}
              onChange={seconds => patch({ scanWarmSlowSeconds: String(seconds) })} />
            <DurationField label={t('settings.scanCold')} seconds={Number(form.scanColdSeconds)} min={1} max={86400}
              onChange={seconds => patch({ scanColdSeconds: String(seconds) })} />
            <DurationField label={t('settings.scanIdleMax')} seconds={Number(form.scanIdleMaxSeconds)} min={1800} max={86400}
              onChange={seconds => patch({ scanIdleMaxSeconds: String(seconds) })} />
            <p className="muted">{t('settings.scanIdleMaxHint')}</p>
            <DurationField label={t('settings.fileStable')} seconds={Number(form.fileStableSeconds)} min={1} max={86400}
              onChange={seconds => patch({ fileStableSeconds: String(seconds) })} />
          </fieldset>
          <fieldset className="settings-group"><legend>{t('settings.backfillGroup')}</legend>
            <label className="checkbox-label"><input type="checkbox" checked={form.autoBackfill} onChange={event => patch({ autoBackfill: event.target.checked })} />{t('settings.autoBackfill')}</label>
            <p className="muted">{t('settings.autoBackfillHint')}</p>
            <DurationField label={t('settings.backfillInterval')} seconds={Number(form.backfillIntervalSeconds)} min={10} max={86400}
              onChange={seconds => patch({ backfillIntervalSeconds: String(seconds) })} />
            <label>{t('settings.backfillDailyLimit')}<input type="number" min={0} max={100000} value={form.backfillDailyLimit} onChange={event => patch({ backfillDailyLimit: event.target.value })} /></label>
            <label>{t('settings.backfillDailyGiB')}<input type="number" min={0} max={102400} value={form.backfillDailyGiB} onChange={event => patch({ backfillDailyGiB: event.target.value })} /></label>
            <label>{t('settings.hashLimit')}<input type="number" min={0} max={102400} value={form.hashLimitMiBps} onChange={event => patch({ hashLimitMiBps: event.target.value })} /></label>
          </fieldset>
          <fieldset className="settings-group"><legend>{t('settings.coverGroup')}</legend>
            <label className="checkbox-label"><input type="checkbox" checked={form.autoCover} onChange={event => patch({ autoCover: event.target.checked })} />{t('settings.autoCover')}</label>
            <p className="muted">{t('settings.autoCoverHint')}</p>
            <DurationField label={t('settings.coverInterval')} seconds={Number(form.coverIntervalSeconds)} min={10} max={86400}
              onChange={seconds => patch({ coverIntervalSeconds: String(seconds) })} />
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
  </div>
  </>;
}

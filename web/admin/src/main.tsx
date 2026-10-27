import { lazy, Suspense, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';
import { api } from './api';
import { Library } from './Library';
import { Login } from './Login';
import { Settings } from './Settings';
import { Monitor } from './Monitor';
import { I18nProvider, useI18n } from './i18n';
import { LiveProvider } from './live';
import { pages, usePage, type Page } from './navigation';

const ApiDocs = lazy(() => import('./ApiDocs'));

type Theme = 'system' | 'light' | 'dark';
type ProtocolInfo = { protocolVersion: number; supportedVersions: number[]; capabilities: string[] };
type RuntimeConfig = {
  source: string;
  network?: { stateFile: string; rememberEnabled: boolean; previousPort: number; activePort: number; saved: boolean; warning?: string };
  effective: { host: string; preferredPort: number; fallbackPorts: number[] | null; dataDirectory: string; webDirectory: string; autoScan: boolean; scanIntervalSeconds: number; fileStableSeconds: number };
};
type Status = {
  status: string;
  listenAddress: string;
  build: {
    service: string; version: string; commit: string; buildTime: string;
    channel: string; dirty: boolean; protocolVersion: number;
    imageBuildTime: string; imageSourceDigest: string;
  };
};

function initialTheme(): Theme {
  try {
    const value = localStorage.getItem('dvn-theme');
    if (value === 'light' || value === 'dark') return value;
  } catch { /* 無儲存權限時仍可使用系統外觀。 */ }
  return 'system';
}

/** formatTime 將建置時間轉為瀏覽器本地時間；無法解析時保留原始值。 */
function formatTime(value: string, locale: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString(locale, { hour12: false });
}

function App() {
  const page = usePage();
  const { t, locale, setLocale, locales } = useI18n();
  const [theme, setTheme] = useState<Theme>(initialTheme);
  const [status, setStatus] = useState<Status | null>(null);
  const [error, setError] = useState('');
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [runtimeConfig, setRuntimeConfig] = useState<RuntimeConfig | null>(null);
  const [configError, setConfigError] = useState('');
  const [nodeId, setNodeId] = useState('');
  const [protocol, setProtocol] = useState<ProtocolInfo | null>(null);
  const [protocolError, setProtocolError] = useState('');

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try { localStorage.setItem('dvn-theme', theme); } catch { /* 外觀仍適用本次工作階段。 */ }
  }, [theme]);

  useEffect(() => {
    document.title = t('app.title');
  }, [t]);

  useEffect(() => {
    const controller = new AbortController();
    fetch('/api/v1/status', { signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw new Error(t('error.serviceResponse', { status: response.status }));
        setStatus(await response.json() as Status);
      })
      .catch((reason: Error) => { if (!controller.signal.aborted) setError(reason.message); });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    let active = true;
    fetch('/api/v1/auth/session', { cache: 'no-store' })
      .then(async response => {
        if (!response.ok) throw new Error(t('error.sessionResponse', { status: response.status }));
        return await response.json() as { authenticated: boolean };
      })
      .then(result => { if (active) setAuthenticated(Boolean(result.authenticated)); })
      .catch(() => { if (active) setAuthenticated(false); });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    const unauthorized = () => setAuthenticated(false);
    window.addEventListener('dvn-unauthorized', unauthorized);
    return () => window.removeEventListener('dvn-unauthorized', unauthorized);
  }, []);

  useEffect(() => {
    if (!authenticated || page !== 'diagnostics') return;
    const controller = new AbortController();
    setConfigError('');
    setRuntimeConfig(null);
    setProtocol(null);
    setProtocolError('');
    api<ProtocolInfo>('protocol', { signal: controller.signal })
      .then(setProtocol)
      .catch((reason: Error) => { if (!controller.signal.aborted) setProtocolError(reason.message); });
    api<RuntimeConfig>('config', { signal: controller.signal })
      .then(setRuntimeConfig)
      .catch((reason: Error) => { if (!controller.signal.aborted) setConfigError(reason.message); });
    return () => controller.abort();
  }, [authenticated, page]);

  async function logout() {
    try { await api('auth/logout', { method: 'POST' }); } catch { /* 已失效的 session 仍應回到登入畫面。 */ }
    setAuthenticated(false);
    setRuntimeConfig(null);
    setConfigError('');
  }

  return <main>
    <header><div><p className="eyebrow">{t('app.eyebrow')}</p><h1>{t('app.title')}</h1></div>
      <div className="header-actions">
        <label>{t('app.locale')}<select className="locale-select" value={locale} onChange={event => setLocale(event.target.value)}>
          {locales.map(item => <option key={item.code} value={item.code}>{item.label}</option>)}
        </select></label>
        <label>{t('app.theme')}<select value={theme} onChange={event => setTheme(event.target.value as Theme)}>
          <option value="system">{t('app.theme.system')}</option><option value="light">{t('app.theme.light')}</option><option value="dark">{t('app.theme.dark')}</option>
        </select></label>
        {authenticated === true && <button onClick={() => void logout()}>{t('app.logout')}</button>}
      </div>
    </header>
    {error && <section><p role="alert">{t('app.unreachable', { error })}</p></section>}
    {!status && !error && <p role="status">{t('app.loadingStatus')}</p>}
    {authenticated === null && <section><p role="status">{t('app.checkingAuth')}</p></section>}
    {authenticated === false && <Login service={status?.build.service} version={status?.build.version}
      onAuthenticated={() => setAuthenticated(true)} />}
    {authenticated === true && <LiveProvider><>
      <nav className="main-nav" aria-label={t('app.navAria')}>
        {(Object.entries(pages) as [Page, string][]).filter(([key]) => !status || status.build.service === 'video-node'
          || key === 'overview' || key === 'monitor' || key === 'api' || key === 'settings' || key === 'diagnostics').map(([key, label]) =>
          <a key={key} href={`#${key}`} aria-current={page === key ? 'page' : undefined}>{t(label)}</a>)}
      </nav>
      <div className="page-heading"><h2>{t(pages[page])}</h2><p className="muted">{status?.build.service || t('app.connecting')}</p></div>
      {page === 'settings' && <Settings service={status?.build.service} />}
      {page === 'monitor' && <Monitor />}
      {page === 'api' && <Suspense fallback={<section><p role="status">{t('app.loadingApiDocs')}</p></section>}><ApiDocs /></Suspense>}
      {status?.build.service === 'video-node' && <Library page={page} onNodeId={setNodeId} />}
      {status && page === 'overview' && <section><h2>{t('app.nodeService')}</h2><p className="badge">{t('app.serviceRunning', { service: status.build.service })}</p><p>{t('app.version', { version: status.build.version })}</p><a href="#diagnostics">{t('app.viewFullInfo')}</a></section>}
      {status && status.build.service !== 'video-node' && page !== 'overview' && page !== 'monitor' && page !== 'api' && page !== 'diagnostics' && page !== 'settings' && <section><p>{t('app.onlyVideoNode')}</p><a href="#overview">{t('app.backOverview')}</a></section>}
      {page === 'diagnostics' && status && <div className="split-layout">
        <section><h2>{t('diagnostics.buildTitle')}</h2>
          <p className="badge">{t('app.serviceRunning', { service: status.build.service })}</p>
          <dl>{Object.entries({
            ...(nodeId ? { [t('diagnostics.nodeId')]: nodeId } : {}),
            [t('diagnostics.version')]: status.build.version, [t('diagnostics.commit')]: status.build.commit,
            [t('diagnostics.programBuildTime')]: formatTime(status.build.buildTime, locale),
            [t('diagnostics.imageBuildTime')]: !status.build.imageBuildTime || status.build.imageBuildTime === 'unknown' ? t('diagnostics.imageBuildMissing')
              : t('diagnostics.imageBuildSource', {
                time: formatTime(status.build.imageBuildTime, locale),
                source: !status.build.imageSourceDigest || status.build.imageSourceDigest === 'unknown' ? t('common.notProvided') : status.build.imageSourceDigest.slice(0, 12),
              }),
            [t('diagnostics.releaseChannel')]: status.build.channel,
            [t('diagnostics.protocolVersion')]: status.build.protocolVersion, [t('diagnostics.dirty')]: status.build.dirty ? t('common.yes') : t('common.no'),
            [t('diagnostics.listen')]: status.listenAddress,
          }).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
          <h3>{t('diagnostics.apiProtocolTitle')}</h3>
          {protocolError && <p role="alert">{protocolError}</p>}
          {protocol && <p>{t('diagnostics.protocolSummary', {
            current: protocol.protocolVersion,
            versions: protocol.supportedVersions.join(t('common.listSeparator')),
            count: protocol.capabilities.length,
          })}</p>}
          <div className="button-row"><a href="#api">{t('diagnostics.openApiDocs')}</a><a href="/api/v1/openapi.json" target="_blank" rel="noreferrer">{t('diagnostics.rawOpenApi')}</a></div>
        </section>
        <section><h2>{t('diagnostics.effectiveTitle')}</h2>
          {configError ? <p role="alert">{configError}</p> : !runtimeConfig ? <p role="status">{t('diagnostics.configLoading')}</p> : <dl className="dl-columns">{Object.entries({
            [t('diagnostics.sourceLabel')]: runtimeConfig.source,
            [t('diagnostics.host')]: runtimeConfig.effective.host,
            [t('diagnostics.preferredPort')]: runtimeConfig.effective.preferredPort,
            [t('diagnostics.fallbackPorts')]: runtimeConfig.effective.fallbackPorts?.join(', ') || t('diagnostics.notEnabled'),
            [t('diagnostics.dataDirectory')]: runtimeConfig.effective.dataDirectory,
            [t('diagnostics.webDirectory')]: runtimeConfig.effective.webDirectory,
            ...(runtimeConfig.network ? {
              [t('diagnostics.rememberPort')]: runtimeConfig.network.rememberEnabled ? t('common.enabled') : t('diagnostics.rememberDisabled'),
              [t('diagnostics.previousPort')]: runtimeConfig.network.previousPort || t('common.noRecord'),
              [t('diagnostics.activePort')]: runtimeConfig.network.activePort,
              [t('diagnostics.stateFile')]: runtimeConfig.network.stateFile,
              [t('diagnostics.stateSaved')]: runtimeConfig.network.saved ? t('common.saved') : t('common.notSaved'),
              ...(runtimeConfig.network.warning ? { [t('diagnostics.stateWarning')]: runtimeConfig.network.warning } : {}),
            } : {}),
            ...(status.build.service === 'video-node' ? {
              [t('diagnostics.autoScan')]: runtimeConfig.effective.autoScan ? t('common.enabled') : t('common.disabled'),
              [t('diagnostics.scanInterval')]: t('common.seconds', { count: runtimeConfig.effective.scanIntervalSeconds }),
              [t('diagnostics.fileStableTime')]: t('common.seconds', { count: runtimeConfig.effective.fileStableSeconds }),
            } : {}),
          }).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>}
        </section>
      </div>}
    </></LiveProvider>}
  </main>;
}

createRoot(document.getElementById('root')!).render(<I18nProvider><App /></I18nProvider>);

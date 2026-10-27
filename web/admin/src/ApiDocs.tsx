import { useEffect, useState } from 'react';
import SwaggerUI from 'swagger-ui-react';
import 'swagger-ui-react/swagger-ui.css';
import './api-docs.css';
import { api } from './api';
import { useI18n } from './i18n';

// React 包裝器未提供 validatorUrl prop，透過外掛移除線上驗證元件，避免外部連線。
const offlineDocsPlugin = () => ({ wrapComponents: { OnlineValidatorBadge: () => () => null } });

/** ApiDocs 以目前登入服務的 OpenAPI 產生可查閱與執行的同源 API 文件。 */
export default function ApiDocs() {
  const { t } = useI18n();
  const [spec, setSpec] = useState<object | null>(null);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setSpec(null); setError('');
    api<object>('openapi.json', { signal: controller.signal })
      .then(value => { if (!controller.signal.aborted) setSpec(value); })
      .catch((reason: Error) => { if (!controller.signal.aborted) setError(reason.message); });
    return () => controller.abort();
  }, [reload]);

  return <>
    <section><div className="section-heading"><h2>{t('apiDocs.title')}</h2><a href="/api/v1/openapi.json" target="_blank" rel="noreferrer">{t('apiDocs.raw')}</a></div>
      <p>{t('apiDocs.intro')}</p>
      <ol><li>{t('apiDocs.step1')}</li><li>{t('apiDocs.step2')}</li><li>{t('apiDocs.step3')}</li></ol>
      <p className="muted">{t('apiDocs.warning')}</p>
      <p className="muted">{t('apiDocs.curlNote')}</p>
      {error && <p role="alert">{t('apiDocs.loadError', { error })} <button onClick={() => setReload(value => value + 1)}>{t('apiDocs.reload')}</button></p>}
      {!spec && !error && <p role="status">{t('apiDocs.loading')}</p>}
    </section>
    {spec && <section className="api-reference" aria-label={t('apiDocs.aria')}>
      <SwaggerUI spec={spec} docExpansion="none" filter defaultModelsExpandDepth={-1}
        plugins={[offlineDocsPlugin]}
        deepLinking={false} queryConfigEnabled={false} tryItOutEnabled={false}
        withCredentials persistAuthorization={false} displayRequestDuration
        supportedSubmitMethods={['get', 'post', 'put', 'delete', 'head']}
        requestInterceptor={request => {
          const target = new URL(String(request.url), window.location.origin);
          if (target.origin !== window.location.origin || !(target.pathname.startsWith('/api/v1/') || target.pathname === '/healthz')) {
            throw new Error(t('apiDocs.onlySameOrigin'));
          }
          request.credentials = 'same-origin';
          request.headers ||= {};
          if (target.pathname.startsWith('/api/') && !Object.keys(request.headers).some(key => key.toLowerCase() === 'x-dvn-protocol-version')) {
            request.headers['X-DVN-Protocol-Version'] = '1';
          }
          return request;
        }}
        responseInterceptor={response => {
          if (response.status === 401 || (response.status === 200 && new URL(String(response.url), window.location.origin).pathname === '/api/v1/auth/logout')) {
            window.dispatchEvent(new Event('dvn-unauthorized'));
          }
          return response;
        }} />
    </section>}
  </>;
}

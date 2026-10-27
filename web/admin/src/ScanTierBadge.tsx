import { useI18n } from './i18n';

type Tier = 'hot' | 'warm' | 'cold' | 'dormant';
type Translate = (key: string, params?: Record<string, string | number>) => string;

const hour = 60 * 60 * 1000;
const day = 24 * hour;

function tierLabel(tier: Tier, t: Translate): string {
  if (tier === 'hot') return t('scanTier.hot');
  if (tier === 'warm') return t('scanTier.warm');
  if (tier === 'cold') return t('scanTier.cold');
  return t('scanTier.dormant');
}

function transition(tier: Tier): { after: number } | null {
  if (tier === 'hot') return { after: hour };
  if (tier === 'warm') return { after: day };
  if (tier === 'cold') return { after: 7 * day };
  return null;
}

function formatTime(value: string, locale: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(locale, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

/** scanTierActivity 回傳上次活動與預估降級時間，供 badge tooltip 與監看列使用。 */
export function scanTierActivity(tier: string | undefined, lastActivityAt: string | undefined, t: Translate, locale: string): string {
  if (tier !== 'hot' && tier !== 'warm' && tier !== 'cold' && tier !== 'dormant') return '';
  if (!lastActivityAt) return t('scanTier.noActivity');
  const last = new Date(lastActivityAt);
  if (Number.isNaN(last.getTime())) return t('scanTier.noActivity');
  const parts = [t('scanTier.lastActivity', { time: formatTime(lastActivityAt, locale) })];
  const next = transition(tier);
  if (next) {
    const boundary = new Date(last.getTime() + next.after);
    const when = formatTime(boundary.toISOString(), locale);
    if (tier === 'hot') parts.push(t('scanTier.turnWarm', { time: when }));
    else if (tier === 'warm') parts.push(t('scanTier.turnCold', { time: when }));
    else parts.push(t('scanTier.turnDormant', { time: when }));
    parts.push(t('scanTier.transitionNote'));
  } else {
    parts.push(t('scanTier.stayDormant'));
  }
  return parts.join(t('common.dot'));
}

/** ScanTierBadge 將掃描分級獨立成有色標籤，避免和上線狀態混在一起。 */
export function ScanTierBadge({ tier, lastActivityAt }: { tier?: string; lastActivityAt?: string }) {
  const { t, locale } = useI18n();
  if (tier !== 'hot' && tier !== 'warm' && tier !== 'cold' && tier !== 'dormant') return null;
  return <span className={`tier-badge ${tier}`} title={scanTierActivity(tier, lastActivityAt, t, locale)}>{tierLabel(tier, t)}</span>;
}

import { useI18n } from './i18n';

/** ScanTierBadge 將掃描分級獨立成有色標籤，避免和上線狀態混在一起。 */
export function ScanTierBadge({ tier }: { tier?: string }) {
  const { t } = useI18n();
  if (tier !== 'hot' && tier !== 'warm' && tier !== 'cold' && tier !== 'dormant') return null;
  const label = tier === 'hot' ? t('scanTier.hot') : tier === 'warm' ? t('scanTier.warm')
    : tier === 'cold' ? t('scanTier.cold') : t('scanTier.dormant');
  return <span className={`tier-badge ${tier}`}>{label}</span>;
}

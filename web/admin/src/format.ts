type Translate = (key: string, params?: Record<string, string | number>) => string;

/** formatScanFrequency 將掃描間隔換算成每小時／每天的約略次數。 */
export function formatScanFrequency(seconds: number, t: Translate): string | null {
  if (!Number.isFinite(seconds) || seconds <= 0) return null;
  const perDay = seconds >= 30 * 60;
  const count = Math.round((perDay ? 86400 / seconds : 3600 / seconds) * 10) / 10;
  return perDay
    ? t('scanFrequency.perDay', { count })
    : t('scanFrequency.perHour', { count });
}


/** formatDuration 將秒數轉成人類可讀的在地化字串；最多顯示兩個單位。 */
export function formatDuration(value: number, t: Translate): string {
  if (!Number.isFinite(value) || value < 0) return t('common.notProvided');
  let remaining = Math.round(value);
  if (remaining === 0) return t('duration.seconds', { value: 0 });
  const parts: string[] = [];
  const days = Math.floor(remaining / 86400);
  if (days > 0) {
    parts.push(t('duration.days', { value: days }));
    remaining -= days * 86400;
  }
  if (parts.length < 2) {
    const hours = Math.floor(remaining / 3600);
    if (hours > 0) {
      parts.push(t('duration.hours', { value: hours }));
      remaining -= hours * 3600;
    }
  }
  if (parts.length < 2) {
    const minutes = Math.floor(remaining / 60);
    if (minutes > 0) {
      parts.push(t('duration.minutes', { value: minutes }));
      remaining -= minutes * 60;
    }
  }
  if (parts.length < 2 && remaining > 0) {
    parts.push(t('duration.seconds', { value: remaining }));
  }
  return parts.join(' ');
}

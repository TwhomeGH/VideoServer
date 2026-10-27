import { useEffect, useRef, useState } from 'react';
import { useI18n } from './i18n';
import { formatDuration } from './format';

type DurationUnit = 'seconds' | 'minutes' | 'hours' | 'days';
type Props = {
  label: string;
  seconds: number;
  onChange: (seconds: number) => void;
  min: number;
  max: number;
};

const unitSeconds: Record<DurationUnit, number> = { seconds: 1, minutes: 60, hours: 3600, days: 86400 };

function bestUnit(seconds: number): DurationUnit {
  if (seconds >= 86400 && seconds % 86400 === 0) return 'days';
  if (seconds >= 3600 && seconds % 3600 === 0) return 'hours';
  if (seconds >= 60 && seconds % 60 === 0) return 'minutes';
  return 'seconds';
}

function unitValue(seconds: number, unit: DurationUnit): string {
  const value = seconds / unitSeconds[unit];
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(3)));
}

/** DurationField 以數字＋單位編輯秒數，表單仍送出整數秒。 */
export function DurationField({ label, seconds, onChange, min, max }: Props) {
  const { t } = useI18n();
  const initial = bestUnit(seconds);
  const [unit, setUnit] = useState<DurationUnit>(initial);
  const [value, setValue] = useState(() => unitValue(seconds, initial));
  const lastEmitted = useRef(seconds);

  useEffect(() => {
    if (seconds === lastEmitted.current) return;
    const nextUnit = bestUnit(seconds);
    setUnit(nextUnit);
    setValue(unitValue(seconds, nextUnit));
    lastEmitted.current = seconds;
  }, [seconds]);

  const emit = (text: string, nextUnit: DurationUnit) => {
    const parsed = Number(text);
    if (!Number.isFinite(parsed) || parsed < 0) return;
    const total = Math.round(parsed * unitSeconds[nextUnit]);
    lastEmitted.current = total;
    onChange(total);
  };
  const changeValue = (text: string) => {
    setValue(text);
    emit(text, unit);
  };
  const changeUnit = (nextUnit: DurationUnit) => {
    const current = Number(value);
    const total = Number.isFinite(current) ? Math.round(current * unitSeconds[unit]) : seconds;
    setUnit(nextUnit);
    setValue(unitValue(total, nextUnit));
    lastEmitted.current = total;
    onChange(total);
  };

  return <label className="duration-label">{label}
    <span className="duration-field">
      <input type="number" step="any" min={min / unitSeconds[unit]} max={max / unitSeconds[unit]} value={value}
        onChange={event => changeValue(event.target.value)} />
      <select value={unit} onChange={event => changeUnit(event.target.value as DurationUnit)}>
        <option value="seconds">{t('duration.unitSeconds')}</option>
        <option value="minutes">{t('duration.unitMinutes')}</option>
        <option value="hours">{t('duration.unitHours')}</option>
        <option value="days">{t('duration.unitDays')}</option>
      </select>
    </span>
    <span className="muted duration-hint">= {formatDuration(seconds, t)}</span>
  </label>;
}

import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { SchedulePayload, Zone, api } from '../api'
import { Stepper, useToast } from '../components'
import { locale, t, weekdays } from '../i18n'
import { fmtTime, nextRunPreview, parseTime } from '../util'

interface TimeSlot {
  enabled: boolean
  value: string
}

const EMPTY: SchedulePayload = {
  name: '',
  enabled: true,
  kind: 'weekly',
  days: [true, true, true, true, true, true, true],
  interval: 2,
  restriction: 0,
  weatherAdjust: false,
  startTimes: [],
  durations: [],
  cycleMaxMinutes: 0,
  soakMinutes: 0,
}

function previewLabel(p: { day: Date; inDays: number; times: number[] }): string {
  const times = p.times.map(fmtTime).join(', ')
  if (p.inDays === 0) return `${t('time.today')} ${times}`
  if (p.inDays === 1) return `${t('time.tomorrow')} ${times}`
  const day = p.day.toLocaleDateString(locale(), {
    weekday: 'short',
    day: '2-digit',
    month: '2-digit',
  })
  return `${day} ${times}`
}

export default function ScheduleEdit() {
  const { id } = useParams()
  const isNew = id === 'new'
  const nav = useNavigate()
  const toast = useToast()

  const [form, setForm] = useState<SchedulePayload>(EMPTY)
  const [slots, setSlots] = useState<TimeSlot[]>(
    Array.from({ length: 4 }, () => ({ enabled: false, value: '06:00' })),
  )
  const [zones, setZones] = useState<Zone[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(isNew)

  useEffect(() => {
    api
      .zones()
      .then((z) => {
        setZones(z.zones)
        if (isNew) {
          setForm((f) => ({ ...f, durations: z.zones.map(() => 0) }))
        }
      })
      .catch((e: Error) => setError(e.message))
    if (!isNew) {
      api
        .schedule(Number(id))
        .then((s) => {
          setForm({
            name: s.name,
            enabled: s.enabled,
            kind: s.kind,
            days: s.days,
            interval: s.interval,
            restriction: s.restriction,
            weatherAdjust: s.weatherAdjust,
            startTimes: s.startTimes,
            durations: s.durations,
            cycleMaxMinutes: s.cycleMaxMinutes,
            soakMinutes: s.soakMinutes,
          })
          setSlots(
            Array.from({ length: 4 }, (_, i) => ({
              enabled: i < s.startTimes.length,
              value: i < s.startTimes.length ? fmtTime(s.startTimes[i]) : '06:00',
            })),
          )
          setLoaded(true)
        })
        .catch((e: Error) => setError(e.message))
    }
  }, [id, isNew])

  const patch = (p: Partial<SchedulePayload>) => setForm((f) => ({ ...f, ...p }))

  const toggleDay = (i: number) => {
    const days = [...form.days]
    days[i] = !days[i]
    patch({ days })
  }

  const setDuration = (zoneId: number, v: number) => {
    const durations = [...form.durations]
    durations[zoneId] = v
    patch({ durations })
  }

  const enabledZones = zones.filter((z) => z.enabled)
  const startTimes = slots
    .filter((s) => s.enabled)
    .map((s) => parseTime(s.value))
    .filter((n): n is number => n !== null)

  // Cheap enough to recompute on every render — no memoization needed.
  const preview = nextRunPreview(
    {
      enabled: form.enabled,
      kind: form.kind,
      days: form.days,
      interval: form.interval,
      restriction: form.restriction,
    },
    startTimes,
    new Date(),
    3,
  )

  const totalMinutes = enabledZones.reduce((sum, z) => sum + (form.durations[z.id] ?? 0), 0)

  const save = async () => {
    for (const s of slots) {
      if (s.enabled && parseTime(s.value) === null) {
        setError(t('edit.invalidTime', { v: s.value }))
        return
      }
    }
    const payload = { ...form, startTimes }
    try {
      if (isNew) await api.createSchedule(payload)
      else await api.updateSchedule(Number(id), payload)
      toast(isNew ? t('edit.created') : t('edit.saved'))
      nav('/schedules')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  if (!loaded && !error) return <p className="muted">{t('common.loading')}</p>

  return (
    <>
      <h1>{isNew ? t('edit.titleNew') : t('edit.titleEdit')}</h1>
      {error && <div className="banner error">{error}</div>}

      <div className="card">
        <label className="field">
          <span>{t('edit.name')}</span>
          <input
            type="text"
            value={form.name}
            style={{ width: '100%', maxWidth: 380 }}
            onChange={(e) => patch({ name: e.target.value })}
          />
        </label>
        <div className="row" style={{ gap: 24 }}>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => patch({ enabled: e.target.checked })}
            />
            {t('edit.active')}
          </label>
          <label className="checkbox" title={t('edit.weatherAdjustTitle')}>
            <input
              type="checkbox"
              checked={form.weatherAdjust}
              onChange={(e) => patch({ weatherAdjust: e.target.checked })}
            />
            {t('edit.weatherAdjust')}
          </label>
        </div>
      </div>

      <div className="card">
        <h2>{t('edit.when')}</h2>
        <div className="row" style={{ marginBottom: 12 }}>
          <label className="checkbox">
            <input
              type="radio"
              name="kind"
              checked={form.kind === 'weekly'}
              onChange={() => patch({ kind: 'weekly' })}
            />
            {t('edit.weekdays')}
          </label>
          <label className="checkbox">
            <input
              type="radio"
              name="kind"
              checked={form.kind === 'interval'}
              onChange={() => patch({ kind: 'interval' })}
            />
            {t('edit.interval')}
          </label>
        </div>

        {form.kind === 'weekly' ? (
          <>
            <div className="daypick" style={{ marginBottom: 12 }}>
              {weekdays().map((d, i) => (
                <button
                  key={i}
                  type="button"
                  className={form.days[i] ? 'sel' : ''}
                  onClick={() => toggleDay(i)}
                >
                  {d}
                </button>
              ))}
            </div>
            <label className="field">
              <span>{t('edit.restriction')}</span>
              <select
                value={form.restriction}
                onChange={(e) => patch({ restriction: Number(e.target.value) })}
              >
                <option value={0}>{t('edit.restrictionNone')}</option>
                <option value={1}>{t('edit.restrictionOdd')}</option>
                <option value={2}>{t('edit.restrictionEven')}</option>
              </select>
            </label>
          </>
        ) : (
          <label className="field">
            <span>{t('edit.everyN')}</span>
            <input
              type="number"
              min={1}
              max={365}
              value={form.interval}
              onChange={(e) => patch({ interval: Number(e.target.value) || 1 })}
            />
          </label>
        )}

        <h2 style={{ marginTop: 16 }}>{t('edit.startTimes')}</h2>
        <div className="row">
          {slots.map((s, i) => (
            <label className="checkbox" key={i} style={{ gap: 4 }}>
              <input
                type="checkbox"
                checked={s.enabled}
                onChange={(e) =>
                  setSlots(slots.map((x, j) => (j === i ? { ...x, enabled: e.target.checked } : x)))
                }
              />
              <input
                type="time"
                value={s.value}
                disabled={!s.enabled}
                onChange={(e) =>
                  setSlots(slots.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))
                }
              />
            </label>
          ))}
        </div>

        <p className="muted small" style={{ marginTop: 12 }}>
          {t('edit.preview')}{' '}
          {preview.length === 0 ? t('edit.previewNone') : preview.map(previewLabel).join(' · ')}
        </p>
      </div>

      <div className="card">
        <div className="row spread">
          <h2>{t('edit.durations')}</h2>
          <span className="muted small">
            {t('edit.total')} <strong>{totalMinutes} min</strong> {t('edit.beforeAdjust')}
          </span>
        </div>
        {enabledZones.map((z) => (
          <div className="zone-row" key={z.id}>
            <span className="name">{z.name}</span>
            <Stepper
              value={form.durations[z.id] ?? 0}
              min={0}
              max={255}
              label={t('edit.zoneRuntimeAria', { name: z.name })}
              onChange={(v) => setDuration(z.id, v)}
            />
          </div>
        ))}
        {enabledZones.length === 0 && <p className="muted">{t('edit.noActiveZones')}</p>}

        <h2 style={{ marginTop: 16 }}>{t('edit.cycleSoak')}</h2>
        <div className="row" style={{ gap: 24 }}>
          <label className="field">
            <span>{t('edit.cycleMax')}</span>
            <input
              type="number"
              min={0}
              max={255}
              value={form.cycleMaxMinutes}
              onChange={(e) => patch({ cycleMaxMinutes: Number(e.target.value) || 0 })}
            />
          </label>
          <label className="field">
            <span>{t('edit.soak')}</span>
            <input
              type="number"
              min={0}
              max={255}
              value={form.soakMinutes}
              disabled={form.cycleMaxMinutes === 0}
              onChange={(e) => patch({ soakMinutes: Number(e.target.value) || 0 })}
            />
          </label>
        </div>
        <p className="muted small">{t('edit.cycleHint')}</p>
      </div>

      <div className="row">
        <button className="primary" onClick={save}>
          {t('common.save')}
        </button>
        <button onClick={() => nav('/schedules')}>{t('common.cancel')}</button>
      </div>
    </>
  )
}

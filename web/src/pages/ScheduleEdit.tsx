import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { SchedulePayload, Zone, api } from '../api'
import { Stepper, useToast } from '../components'
import { WEEKDAYS, fmtTime, nextRunPreview, parseTime } from '../util'

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
}

function previewLabel(p: { day: Date; inDays: number; times: number[] }): string {
  const times = p.times.map(fmtTime).join(', ')
  if (p.inDays === 0) return `Heute ${times}`
  if (p.inDays === 1) return `Morgen ${times}`
  const day = p.day.toLocaleDateString('de-DE', {
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
        setError(`Ungültige Startzeit: ${s.value}`)
        return
      }
    }
    const payload = { ...form, startTimes }
    try {
      if (isNew) await api.createSchedule(payload)
      else await api.updateSchedule(Number(id), payload)
      toast(isNew ? 'Programm angelegt.' : 'Programm gespeichert.')
      nav('/schedules')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  if (!loaded && !error) return <p className="muted">Lade…</p>

  return (
    <>
      <h1>{isNew ? 'Neues Programm' : 'Programm bearbeiten'}</h1>
      {error && <div className="banner error">{error}</div>}

      <div className="card">
        <label className="field">
          <span>Name</span>
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
            Programm aktiv
          </label>
          <label className="checkbox" title="Laufzeiten anhand der Wetterdaten skalieren">
            <input
              type="checkbox"
              checked={form.weatherAdjust}
              onChange={(e) => patch({ weatherAdjust: e.target.checked })}
            />
            Wetter-Anpassung
          </label>
        </div>
      </div>

      <div className="card">
        <h2>Wann</h2>
        <div className="row" style={{ marginBottom: 12 }}>
          <label className="checkbox">
            <input
              type="radio"
              name="kind"
              checked={form.kind === 'weekly'}
              onChange={() => patch({ kind: 'weekly' })}
            />
            Wochentage
          </label>
          <label className="checkbox">
            <input
              type="radio"
              name="kind"
              checked={form.kind === 'interval'}
              onChange={() => patch({ kind: 'interval' })}
            />
            Intervall
          </label>
        </div>

        {form.kind === 'weekly' ? (
          <>
            <div className="daypick" style={{ marginBottom: 12 }}>
              {WEEKDAYS.map((d, i) => (
                <button
                  key={d}
                  type="button"
                  className={form.days[i] ? 'sel' : ''}
                  onClick={() => toggleDay(i)}
                >
                  {d}
                </button>
              ))}
            </div>
            <label className="field">
              <span>Einschränkung (Tag im Monat)</span>
              <select
                value={form.restriction}
                onChange={(e) => patch({ restriction: Number(e.target.value) })}
              >
                <option value={0}>keine</option>
                <option value={1}>nur ungerade Tage</option>
                <option value={2}>nur gerade Tage</option>
              </select>
            </label>
          </>
        ) : (
          <label className="field">
            <span>Alle N Tage</span>
            <input
              type="number"
              min={1}
              max={365}
              value={form.interval}
              onChange={(e) => patch({ interval: Number(e.target.value) || 1 })}
            />
          </label>
        )}

        <h2 style={{ marginTop: 16 }}>Startzeiten (bis zu 4)</h2>
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
          Nächste Läufe:{' '}
          {preview.length === 0
            ? 'keine (Programm aus, keine Startzeit oder kein passender Tag)'
            : preview.map(previewLabel).join(' · ')}
        </p>
      </div>

      <div className="card">
        <div className="row spread">
          <h2>Laufzeit je Zone (Minuten, 0 = überspringen)</h2>
          <span className="muted small">
            Gesamt: <strong>{totalMinutes} min</strong> vor Anpassungen
          </span>
        </div>
        {enabledZones.map((z) => (
          <div className="zone-row" key={z.id}>
            <span className="name">{z.name}</span>
            <Stepper
              value={form.durations[z.id] ?? 0}
              min={0}
              max={255}
              label={`Laufzeit ${z.name}`}
              onChange={(v) => setDuration(z.id, v)}
            />
          </div>
        ))}
        {enabledZones.length === 0 && (
          <p className="muted">Keine aktiven Zonen — erst unter „Zonen&quot; welche aktivieren.</p>
        )}
      </div>

      <div className="row">
        <button className="primary" onClick={save}>
          Speichern
        </button>
        <button onClick={() => nav('/schedules')}>Abbrechen</button>
      </div>
    </>
  )
}

import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, SchedulePayload, Zone } from '../api'
import { WEEKDAYS, fmtTime, parseTime } from '../util'

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

export default function ScheduleEdit() {
  const { id } = useParams()
  const isNew = id === 'new'
  const nav = useNavigate()

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

  const setDuration = (zoneId: number, v: string) => {
    const durations = [...form.durations]
    durations[zoneId] = Math.max(0, Math.min(255, Number(v) || 0))
    patch({ durations })
  }

  const save = async () => {
    const startTimes: number[] = []
    for (const s of slots) {
      if (!s.enabled) continue
      const min = parseTime(s.value)
      if (min === null) {
        setError(`Ungültige Startzeit: ${s.value}`)
        return
      }
      startTimes.push(min)
    }
    const payload = { ...form, startTimes }
    try {
      if (isNew) await api.createSchedule(payload)
      else await api.updateSchedule(Number(id), payload)
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
      </div>

      <div className="card">
        <h2>Laufzeit je Zone (Minuten, 0 = überspringen)</h2>
        {zones
          .filter((z) => z.enabled)
          .map((z) => (
            <div className="zone-row" key={z.id}>
              <span className="name">{z.name}</span>
              <input
                type="number"
                min={0}
                max={255}
                value={form.durations[z.id] ?? 0}
                onChange={(e) => setDuration(z.id, e.target.value)}
              />
            </div>
          ))}
        {zones.filter((z) => z.enabled).length === 0 && (
          <p className="muted">Keine aktiven Zonen — erst unter „Zonen" welche aktivieren.</p>
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

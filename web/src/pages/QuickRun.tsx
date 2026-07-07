import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, Schedule, Zone } from '../api'

export default function QuickRun() {
  const [zones, setZones] = useState<Zone[]>([])
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [durations, setDurations] = useState<Record<number, number>>({})
  const [selected, setSelected] = useState<number>(-1)
  const [error, setError] = useState<string | null>(null)
  const nav = useNavigate()

  useEffect(() => {
    api
      .zones()
      .then((z) => setZones(z.zones))
      .catch((e: Error) => setError(e.message))
    api
      .schedules()
      .then((s) => {
        setSchedules(s.schedules)
        if (s.schedules.length > 0) setSelected(s.schedules[0].id)
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  const startSchedule = async () => {
    try {
      await api.quickRunSchedule(selected)
      nav('/')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const startDurations = async () => {
    const list = zones.map((z) => durations[z.id] ?? 0)
    if (list.every((d) => d === 0)) {
      setError('Mindestens eine Zone braucht eine Laufzeit > 0.')
      return
    }
    try {
      await api.quickRunDurations(list)
      nav('/')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const enabledZones = zones.filter((z) => z.enabled)

  return (
    <>
      <h1>Schnellstart</h1>
      {error && <div className="banner error">{error}</div>}
      <p className="muted">
        Startet sofort eine Bewässerung und unterbricht dabei einen laufenden Zyklus.
      </p>

      <div className="card">
        <h2>Programm sofort starten</h2>
        {schedules.length === 0 ? (
          <p className="muted">Keine Programme vorhanden.</p>
        ) : (
          <div className="row">
            <select value={selected} onChange={(e) => setSelected(Number(e.target.value))}>
              {schedules.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
            <button className="primary" onClick={startSchedule}>
              Starten
            </button>
          </div>
        )}
        <p className="muted small">Saisonale und Wetter-Anpassung werden angewendet.</p>
      </div>

      <div className="card">
        <h2>Eigene Laufzeiten (Minuten)</h2>
        {enabledZones.length === 0 ? (
          <p className="muted">Keine aktiven Zonen.</p>
        ) : (
          <>
            {enabledZones.map((z) => (
              <div className="zone-row" key={z.id}>
                <span className="name">{z.name}</span>
                <input
                  type="number"
                  min={0}
                  max={255}
                  value={durations[z.id] ?? 0}
                  onChange={(e) =>
                    setDurations({
                      ...durations,
                      [z.id]: Math.max(0, Math.min(255, Number(e.target.value) || 0)),
                    })
                  }
                />
              </div>
            ))}
            <div className="row" style={{ marginTop: 12 }}>
              <button className="primary" onClick={startDurations}>
                Starten
              </button>
            </div>
            <p className="muted small">Läuft ohne Anpassungen, Zonen nacheinander.</p>
          </>
        )}
      </div>
    </>
  )
}

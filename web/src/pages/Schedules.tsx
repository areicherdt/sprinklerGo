import { Link, useNavigate } from 'react-router-dom'
import { api, Schedule } from '../api'
import { WEEKDAYS, fmtNextRun, fmtTime, usePoll } from '../util'

function daysLabel(s: Schedule): string {
  if (s.kind === 'interval') return `alle ${s.interval} Tage`
  const days = WEEKDAYS.filter((_, i) => s.days[i])
  const label = days.length === 7 ? 'täglich' : days.join(', ')
  if (s.restriction === 1) return `${label} (ungerade Tage)`
  if (s.restriction === 2) return `${label} (gerade Tage)`
  return label
}

export default function Schedules() {
  const [data, refresh, error] = usePoll(() => api.schedules(), 10000)
  const nav = useNavigate()

  const remove = async (id: number, name: string) => {
    if (!window.confirm(`Programm „${name}" wirklich löschen?`)) return
    await api.deleteSchedule(id).catch(() => {})
    refresh()
  }

  const runNow = async (id: number) => {
    await api.quickRunSchedule(id).catch((e: Error) => window.alert(e.message))
    nav('/')
  }

  return (
    <>
      <div className="row spread">
        <h1>Programme</h1>
        <Link to="/schedules/new">
          <button className="primary">Neues Programm</button>
        </Link>
      </div>
      {error && <div className="banner error">Keine Verbindung zum Server: {error}</div>}

      {data && data.schedules.length === 0 && (
        <div className="card">
          <p className="muted">Noch keine Programme angelegt.</p>
        </div>
      )}

      {data?.schedules.map((s) => (
        <div className="card" key={s.id}>
          <div className="row spread">
            <div>
              <div className="row">
                <h2 style={{ margin: 0 }}>{s.name}</h2>
                <span className={`pill ${s.enabled ? 'on' : 'off'}`}>
                  {s.enabled ? 'aktiv' : 'aus'}
                </span>
                {s.weatherAdjust && <span className="pill">Wetter-Anpassung</span>}
              </div>
              <p className="muted small" style={{ margin: '6px 0 0' }}>
                {daysLabel(s)} · Start: {s.startTimes.map(fmtTime).join(', ') || '—'}
                <br />
                Nächster Lauf: {fmtNextRun(s.nextRun)}
              </p>
            </div>
            <div className="row">
              <button onClick={() => runNow(s.id)}>Jetzt starten</button>
              <Link to={`/schedules/${s.id}`}>
                <button>Bearbeiten</button>
              </Link>
              <button className="danger" onClick={() => remove(s.id, s.name)}>
                Löschen
              </button>
            </div>
          </div>
        </div>
      ))}
    </>
  )
}

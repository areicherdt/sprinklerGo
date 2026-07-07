import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Schedule, api } from '../api'
import { ConfirmDialog, useToast } from '../components'
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
  const [toDelete, setToDelete] = useState<Schedule | null>(null)
  const nav = useNavigate()
  const toast = useToast()

  const remove = async () => {
    if (!toDelete) return
    try {
      await api.deleteSchedule(toDelete.id)
      toast(`Programm „${toDelete.name}" gelöscht.`)
    } catch (e) {
      toast((e as Error).message, 'error')
    }
    setToDelete(null)
    refresh()
  }

  const duplicate = async (s: Schedule) => {
    try {
      const full = await api.schedule(s.id)
      await api.createSchedule({
        name: `${full.name} (Kopie)`,
        enabled: false,
        kind: full.kind,
        days: full.days,
        interval: full.interval,
        restriction: full.restriction,
        weatherAdjust: full.weatherAdjust,
        startTimes: full.startTimes,
        durations: full.durations,
      })
      toast(`Programm „${s.name}" dupliziert (Kopie ist deaktiviert).`)
      refresh()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const runNow = async (s: Schedule) => {
    try {
      await api.quickRunSchedule(s.id)
      toast(`Programm „${s.name}" gestartet.`)
      nav('/')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
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
          <p className="muted">
            Noch keine Programme angelegt. <Link to="/schedules/new">Jetzt das erste anlegen</Link>{' '}
            — oder unter <Link to="/quickrun">Schnellstart</Link> direkt bewässern.
          </p>
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
              <button onClick={() => runNow(s)}>Jetzt starten</button>
              <Link to={`/schedules/${s.id}`}>
                <button>Bearbeiten</button>
              </Link>
              <button onClick={() => duplicate(s)}>Duplizieren</button>
              <button className="danger" onClick={() => setToDelete(s)}>
                Löschen
              </button>
            </div>
          </div>
        </div>
      ))}

      <ConfirmDialog
        open={toDelete !== null}
        title="Programm löschen?"
        text={`„${toDelete?.name}" wird endgültig gelöscht. Bereits protokollierte Läufe bleiben im Verlauf erhalten.`}
        onConfirm={remove}
        onCancel={() => setToDelete(null)}
      />
    </>
  )
}

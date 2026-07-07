import { Link } from 'react-router-dom'
import { api } from '../api'
import { fmtNextRun, fmtSeconds, usePoll } from '../util'

export default function Dashboard() {
  const [state, refresh, error] = usePoll(() => api.state(), 3000)
  const [scheds] = usePoll(() => api.schedules(), 10000)

  const toggleRun = async (enabled: boolean) => {
    await api.setRun(enabled).catch(() => {})
    refresh()
  }
  const stop = async () => {
    await api.stop().catch(() => {})
    refresh()
  }

  const running = state && state.mode !== 'idle'
  const nextRuns = (scheds?.schedules ?? [])
    .filter((s) => s.enabled && s.nextRun)
    .sort((a, b) => {
      const ka = `${a.nextRun!.date} ${String(a.nextRun!.times[0]).padStart(4, '0')}`
      const kb = `${b.nextRun!.date} ${String(b.nextRun!.times[0]).padStart(4, '0')}`
      return ka.localeCompare(kb)
    })

  return (
    <>
      <h1>Übersicht</h1>
      {error && <div className="banner error">Keine Verbindung zum Server: {error}</div>}

      <div className="card-grid">
        <div className="card">
          <div className="row spread">
            <h2>Status</h2>
            {state && (
              <span className={`pill ${running ? 'run' : state.schedulerEnabled ? 'on' : 'off'}`}>
                <span className="dot" />
                {running
                  ? 'Bewässerung läuft'
                  : state.schedulerEnabled
                    ? 'Bereit'
                    : 'Zeitpläne aus'}
              </span>
            )}
          </div>
          {state && running ? (
            <>
              <div className="hero-number">{state.zoneName}</div>
              <p className="muted">
                {state.mode === 'manual' ? (
                  'Manueller Betrieb — läuft bis zum Stopp'
                ) : (
                  <>
                    Programm „{state.scheduleName}" — noch {fmtSeconds(state.remainingSeconds)}
                  </>
                )}
              </p>
              <button className="danger" onClick={stop}>
                Alles stoppen
              </button>
            </>
          ) : (
            <p className="muted">Keine Zone aktiv.</p>
          )}
        </div>

        <div className="card">
          <h2>Automatik</h2>
          {state && (
            <div className="row spread">
              <span>Zeitpläne ausführen</span>
              <label className="switch">
                <input
                  type="checkbox"
                  checked={state.schedulerEnabled}
                  onChange={(e) => toggleRun(e.target.checked)}
                />
                <span className="slider" />
              </label>
            </div>
          )}
          {state && (
            <p className="muted small">
              {state.enabledZones} aktive Zonen · {state.scheduleCount} Programme ·{' '}
              {state.pendingEvents} anstehende Ereignisse · v{state.version}
            </p>
          )}
        </div>
      </div>

      <div className="card">
        <div className="row spread">
          <h2>Nächste Läufe</h2>
          <Link to="/schedules">Programme verwalten</Link>
        </div>
        {nextRuns.length === 0 ? (
          <p className="muted">
            Keine anstehenden Läufe.{' '}
            {state && !state.schedulerEnabled && 'Die Automatik ist ausgeschaltet.'}
          </p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Programm</th>
                  <th>Nächster Lauf</th>
                </tr>
              </thead>
              <tbody>
                {nextRuns.map((s) => (
                  <tr key={s.id}>
                    <td>
                      <Link to={`/schedules/${s.id}`}>{s.name}</Link>
                    </td>
                    <td>{fmtNextRun(s.nextRun)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  )
}

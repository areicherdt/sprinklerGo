import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { LogEntry, Schedule, api } from '../api'
import { useToast } from '../components'
import { ticking, useLiveState, useNowSecond } from '../live'
import { fmtClock, fmtNextRun, fmtSeconds, scheduleLabel } from '../util'

interface TimelineRow {
  at: number
  status: 'done' | 'run' | 'plan'
  label: string
  info: string
}

export default function Dashboard() {
  const { state, receivedAt, error } = useLiveState()
  const now = useNowSecond()
  const toast = useToast()
  const [scheds, setScheds] = useState<Schedule[]>([])
  const [completed, setCompleted] = useState<LogEntry[]>([])

  // Schedules and today's finished runs; refreshed whenever a run starts or
  // ends (mode flips) so the timeline stays current.
  const mode = state?.mode
  useEffect(() => {
    api
      .schedules()
      .then((r) => setScheds(r.schedules))
      .catch(() => {})
    const midnight = new Date()
    midnight.setHours(0, 0, 0, 0)
    api
      .logEntries(Math.floor(midnight.getTime() / 1000), Math.floor(Date.now() / 1000) + 60)
      .then((r) => setCompleted(r.entries))
      .catch(() => {})
  }, [mode])

  const act = (fn: Promise<unknown>, okMsg: string) =>
    fn.then(() => toast(okMsg)).catch((e: Error) => toast(e.message, 'error'))

  const running = state && state.mode !== 'idle'
  const remaining = state ? ticking(state.remainingSeconds, receivedAt, now) : 0
  const rainDelayActive = !!state && state.rainDelayUntil > 0 && state.rainDelayUntil * 1000 > now

  const schedNames = useMemo(() => new Map(scheds.map((s) => [s.id, s.name])), [scheds])

  const timeline = useMemo<TimelineRow[]>(() => {
    if (!state) return []
    const rows: TimelineRow[] = []
    // Finished runs, aggregated per trigger.
    const agg = new Map<number, { at: number; seconds: number; zones: number }>()
    for (const e of completed) {
      const cur = agg.get(e.scheduleId) ?? { at: e.start, seconds: 0, zones: 0 }
      cur.at = Math.min(cur.at, e.start)
      cur.seconds += e.seconds
      cur.zones++
      agg.set(e.scheduleId, cur)
    }
    for (const [id, a] of agg) {
      rows.push({
        at: a.at,
        status: 'done',
        label: scheduleLabel(id, schedNames),
        info: `${a.zones} ${a.zones === 1 ? 'Zone' : 'Zonen'}, ${fmtSeconds(a.seconds)}`,
      })
    }
    if (state.mode === 'schedule' && state.queue.length > 0) {
      const done = state.queue.filter((q) => q.done).length + 1
      rows.push({
        at: state.queue[0].start,
        status: 'run',
        label: state.scheduleName ?? 'Programm',
        info: `läuft — Zone ${done}/${state.queue.length}`,
      })
    }
    if (state.mode === 'manual') {
      rows.push({ at: state.time, status: 'run', label: `${state.zoneName}`, info: 'manuell' })
    }
    for (const p of state.planned) {
      rows.push({ at: p.at, status: 'plan', label: p.scheduleName, info: 'geplant' })
    }
    return rows.sort((a, b) => a.at - b.at)
  }, [state, completed, schedNames])

  const nextRuns = scheds
    .filter((s) => s.enabled && s.nextRun)
    .sort((a, b) =>
      `${a.nextRun!.date} ${String(a.nextRun!.times[0]).padStart(4, '0')}`.localeCompare(
        `${b.nextRun!.date} ${String(b.nextRun!.times[0]).padStart(4, '0')}`,
      ),
    )

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
                  remaining < 0 ? (
                    'Manuell — läuft bis zum Stopp'
                  ) : (
                    <>Manuell — noch {fmtSeconds(remaining)}</>
                  )
                ) : (
                  <>
                    Programm „{state.scheduleName}&quot; — Zone noch {fmtSeconds(remaining)}
                  </>
                )}
              </p>
              {state.mode === 'schedule' && state.queue.length > 0 && (
                <div
                  className="progress"
                  role="img"
                  aria-label="Fortschritt der Zonen des laufenden Programms"
                >
                  {state.queue.map((q) => (
                    <div
                      key={q.zoneId}
                      className={`seg ${q.done ? 'done' : q.active ? 'active' : ''}`}
                      style={{ flexGrow: Math.max(1, q.end - q.start) }}
                      title={q.zoneName}
                    />
                  ))}
                </div>
              )}
              <button className="danger" onClick={() => act(api.stop(), 'Bewässerung gestoppt.')}>
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
                  onChange={(e) =>
                    act(
                      api.setRun(e.target.checked),
                      e.target.checked ? 'Automatik eingeschaltet.' : 'Automatik ausgeschaltet.',
                    )
                  }
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

        <div className="card">
          <h2>Regenpause</h2>
          {state &&
            (rainDelayActive ? (
              <>
                <p>
                  Aktiv bis{' '}
                  <strong>
                    {new Date(state.rainDelayUntil * 1000).toLocaleDateString('de-DE', {
                      weekday: 'short',
                      day: '2-digit',
                      month: '2-digit',
                    })}{' '}
                    {fmtClock(state.rainDelayUntil)}
                  </strong>
                  {' — '}Programme starten nicht, manuelle Bewässerung bleibt möglich.
                </p>
                <button onClick={() => act(api.rainDelay(0), 'Regenpause aufgehoben.')}>
                  Aufheben
                </button>
              </>
            ) : (
              <div className="row">
                {[24, 48, 72].map((h) => (
                  <button
                    key={h}
                    onClick={() => act(api.rainDelay(h), `Regenpause für ${h} h aktiviert.`)}
                  >
                    {h} h
                  </button>
                ))}
              </div>
            ))}
          {!rainDelayActive && (
            <p className="muted small">Setzt Programmstarts vorübergehend aus.</p>
          )}
        </div>

        <div className="card">
          <h2>Wetter</h2>
          {state &&
            (state.weather.provider === 'none' ? (
              <p className="muted">
                Kein Wetter-Anbieter konfiguriert. <Link to="/settings">Jetzt einrichten</Link>
              </p>
            ) : (
              <>
                <div className="hero-number">{state.weather.scale} %</div>
                <p className="muted small">
                  Aktuelle Laufzeit-Skalierung ({state.weather.provider})
                  {state.weather.fetchedAt > 0 && <> · Stand {fmtClock(state.weather.fetchedAt)}</>}
                  {!state.weather.valid && ' · keine gültigen Daten, es gilt 100 %'}
                </p>
              </>
            ))}
        </div>
      </div>

      <div className="card">
        <h2>Heute</h2>
        {timeline.length === 0 ? (
          <p className="muted">
            Heute {state?.schedulerEnabled ? 'keine Läufe' : '— die Automatik ist ausgeschaltet'}.
          </p>
        ) : (
          <ul className="timeline">
            {timeline.map((row, i) => (
              <li key={i} className={row.status}>
                <span className="time">{fmtClock(row.at)}</span>
                <span className="dot" />
                <span className="label">{row.label}</span>
                <span className="muted small">{row.info}</span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="card">
        <div className="row spread">
          <h2>Nächste Läufe</h2>
          <Link to="/schedules">Programme verwalten</Link>
        </div>
        {nextRuns.length === 0 ? (
          <p className="muted">Keine anstehenden Läufe.</p>
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

import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { LogEntry, Schedule, api } from '../api'
import { useToast } from '../components'
import { locale, t } from '../i18n'
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
        info:
          a.zones === 1
            ? t('dash.doneInfoOne', { dur: fmtSeconds(a.seconds) })
            : t('dash.doneInfo', { zones: a.zones, dur: fmtSeconds(a.seconds) }),
      })
    }
    if ((state.mode === 'schedule' || state.mode === 'soaking') && state.queue.length > 0) {
      const done = state.queue.filter((q) => q.done).length
      rows.push({
        at: state.queue[0].start,
        status: 'run',
        label: state.scheduleName ?? t('dash.program'),
        info:
          state.mode === 'soaking'
            ? t('dash.soakingCycles', { a: done, b: state.queue.length })
            : t('dash.runningCycle', {
                a: Math.min(done + 1, state.queue.length),
                b: state.queue.length,
              }),
      })
    }
    if (state.mode === 'manual') {
      rows.push({
        at: state.time,
        status: 'run',
        label: `${state.zoneName}`,
        info: t('dash.manualInfo'),
      })
    }
    for (const p of state.planned) {
      rows.push({
        at: p.at,
        status: 'plan',
        label: p.scheduleName,
        info: p.waiting ? t('dash.waiting') : t('dash.planned'),
      })
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
      <h1>{t('dash.title')}</h1>
      {error && <div className="banner error">{t('common.noConnection', { msg: error })}</div>}

      <div className="card-grid">
        <div className="card">
          <div className="row spread">
            <h2>{t('dash.status')}</h2>
            {state && (
              <span className={`pill ${running ? 'run' : state.schedulerEnabled ? 'on' : 'off'}`}>
                <span className="dot" />
                {running
                  ? t('dash.running')
                  : state.schedulerEnabled
                    ? t('dash.ready')
                    : t('dash.schedulesOff')}
              </span>
            )}
          </div>
          {state && running ? (
            <>
              <div className="hero-number">
                {state.mode === 'soaking' ? t('dash.soaking') : state.zoneName}
              </div>
              <p className="muted">
                {state.mode === 'manual'
                  ? remaining < 0
                    ? t('dash.manualUntilStop')
                    : t('dash.manualRemaining', { t: fmtSeconds(remaining) })
                  : state.mode === 'soaking'
                    ? t('dash.soakingNext', {
                        name: state.scheduleName ?? '',
                        t: fmtSeconds(remaining),
                      })
                    : t('dash.zoneRemaining', {
                        name: state.scheduleName ?? '',
                        t: fmtSeconds(remaining),
                      })}
              </p>
              {(state.mode === 'schedule' || state.mode === 'soaking') &&
                state.queue.length > 0 && (
                  <div className="progress" role="img" aria-label={t('dash.progressAria')}>
                    {state.queue.map((q, i) => (
                      <div
                        key={i}
                        className={`seg ${q.done ? 'done' : q.active ? 'active' : ''}`}
                        style={{ flexGrow: Math.max(1, q.end - q.start) }}
                        title={q.zoneName}
                      />
                    ))}
                  </div>
                )}
              <button className="danger" onClick={() => act(api.stop(), t('dash.stopped'))}>
                {t('dash.stopAll')}
              </button>
            </>
          ) : (
            <p className="muted">{t('dash.noZoneActive')}</p>
          )}
        </div>

        <div className="card">
          <h2>{t('dash.automatic')}</h2>
          {state && (
            <div className="row spread">
              <span>{t('dash.runSchedules')}</span>
              <label className="switch">
                <input
                  type="checkbox"
                  checked={state.schedulerEnabled}
                  onChange={(e) =>
                    act(
                      api.setRun(e.target.checked),
                      e.target.checked ? t('dash.autoOn') : t('dash.autoOff'),
                    )
                  }
                />
                <span className="slider" />
              </label>
            </div>
          )}
          {state && (
            <p className="muted small">
              {t('dash.stats', {
                z: state.enabledZones,
                p: state.scheduleCount,
                e: state.pendingEvents,
              })}{' '}
              · v{state.version}
            </p>
          )}
        </div>

        <div className="card">
          <h2>{t('dash.rainDelay')}</h2>
          {state &&
            (rainDelayActive ? (
              <>
                <p>
                  {t('dash.rainActiveUntil')}{' '}
                  <strong>
                    {new Date(state.rainDelayUntil * 1000).toLocaleDateString(locale(), {
                      weekday: 'short',
                      day: '2-digit',
                      month: '2-digit',
                    })}{' '}
                    {fmtClock(state.rainDelayUntil)}
                  </strong>
                  {t('dash.rainNoStarts')}
                </p>
                <button onClick={() => act(api.rainDelay(0), t('dash.rainCleared'))}>
                  {t('dash.rainClear')}
                </button>
              </>
            ) : (
              <div className="row">
                {[24, 48, 72].map((h) => (
                  <button key={h} onClick={() => act(api.rainDelay(h), t('dash.rainSet', { h }))}>
                    {h} h
                  </button>
                ))}
              </div>
            ))}
          {!rainDelayActive && <p className="muted small">{t('dash.rainHint')}</p>}
        </div>

        <div className="card">
          <h2>{t('dash.weather')}</h2>
          {state &&
            (state.weather.provider === 'none' ? (
              <p className="muted">
                {t('dash.weatherNone')} <Link to="/settings">{t('dash.weatherSetup')}</Link>
              </p>
            ) : (
              <>
                <div className="hero-number">{state.weather.scale} %</div>
                <p className="muted small">
                  {t('dash.weatherScaleLine', { provider: state.weather.provider })}
                  {state.weather.fetchedAt > 0 &&
                    t('dash.weatherAsOf', { t: fmtClock(state.weather.fetchedAt) })}
                  {!state.weather.valid && t('dash.weatherInvalid')}
                </p>
              </>
            ))}
        </div>
      </div>

      <div className="card">
        <h2>{t('dash.today')}</h2>
        {timeline.length === 0 ? (
          <p className="muted">
            {state?.schedulerEnabled ? t('dash.todayNone') : t('dash.todayAutoOff')}
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
          <h2>{t('dash.nextRuns')}</h2>
          <Link to="/schedules">{t('dash.manage')}</Link>
        </div>
        {nextRuns.length === 0 ? (
          <p className="muted">{t('dash.noUpcoming')}</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t('dash.program')}</th>
                  <th>{t('dash.nextRun')}</th>
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

import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Schedule, api } from '../api'
import { ConfirmDialog, useToast } from '../components'
import { t, weekdays } from '../i18n'
import { fmtNextRun, fmtTime, usePoll } from '../util'

function daysLabel(s: Schedule): string {
  if (s.kind === 'interval') return t('sched.everyNDays', { n: s.interval })
  const wd = weekdays()
  const days = wd.filter((_, i) => s.days[i])
  const label = days.length === 7 ? t('sched.daily') : days.join(', ')
  if (s.restriction === 1) return t('sched.oddDays', { days: label })
  if (s.restriction === 2) return t('sched.evenDays', { days: label })
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
      toast(t('sched.deleted', { name: toDelete.name }))
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
        name: `${full.name}${t('sched.copySuffix')}`,
        enabled: false,
        kind: full.kind,
        days: full.days,
        interval: full.interval,
        restriction: full.restriction,
        weatherAdjust: full.weatherAdjust,
        startTimes: full.startTimes,
        durations: full.durations,
        cycleMaxMinutes: full.cycleMaxMinutes,
        soakMinutes: full.soakMinutes,
      })
      toast(t('sched.duplicated', { name: s.name }))
      refresh()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const runNow = async (s: Schedule) => {
    try {
      await api.quickRunSchedule(s.id)
      toast(t('sched.started', { name: s.name }))
      nav('/')
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  return (
    <>
      <div className="row spread">
        <h1>{t('sched.title')}</h1>
        <Link to="/schedules/new">
          <button className="primary">{t('sched.new')}</button>
        </Link>
      </div>
      {error && <div className="banner error">{t('common.noConnection', { msg: error })}</div>}

      {data && data.schedules.length === 0 && (
        <div className="card">
          <p className="muted">
            {t('sched.noneYet')} <Link to="/schedules/new">{t('sched.createFirst')}</Link>{' '}
            {t('sched.orQuickRun', { link: '' })}
            <Link to="/quickrun">{t('nav.quickrun')}</Link>.
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
                  {s.enabled ? t('sched.activePill') : t('sched.offPill')}
                </span>
                {s.weatherAdjust && <span className="pill">{t('sched.weatherPill')}</span>}
              </div>
              <p className="muted small" style={{ margin: '6px 0 0' }}>
                {daysLabel(s)} · {t('sched.start')} {s.startTimes.map(fmtTime).join(', ') || '—'}
                <br />
                {t('sched.nextRun')} {fmtNextRun(s.nextRun)}
              </p>
            </div>
            <div className="row">
              <button onClick={() => runNow(s)}>{t('sched.runNow')}</button>
              <Link to={`/schedules/${s.id}`}>
                <button>{t('common.edit')}</button>
              </Link>
              <button onClick={() => duplicate(s)}>{t('common.duplicate')}</button>
              <button className="danger" onClick={() => setToDelete(s)}>
                {t('common.delete')}
              </button>
            </div>
          </div>
        </div>
      ))}

      <ConfirmDialog
        open={toDelete !== null}
        title={t('sched.confirmTitle')}
        text={t('sched.confirmText', { name: toDelete?.name ?? '' })}
        confirmLabel={t('common.delete')}
        onConfirm={remove}
        onCancel={() => setToDelete(null)}
      />
    </>
  )
}

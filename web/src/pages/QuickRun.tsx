import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, Schedule, Zone } from '../api'
import { useToast } from '../components'
import { t } from '../i18n'

export default function QuickRun() {
  const [zones, setZones] = useState<Zone[]>([])
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [durations, setDurations] = useState<Record<number, number>>({})
  const [selected, setSelected] = useState<number>(-1)
  const [error, setError] = useState<string | null>(null)
  const nav = useNavigate()
  const toast = useToast()

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
      toast(t('quick.started'))
      nav('/')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const startDurations = async () => {
    const list = zones.map((z) => durations[z.id] ?? 0)
    if (list.every((d) => d === 0)) {
      setError(t('quick.needOne'))
      return
    }
    try {
      await api.quickRunDurations(list)
      toast(t('quick.started'))
      nav('/')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const enabledZones = zones.filter((z) => z.enabled)

  return (
    <>
      <h1>{t('quick.title')}</h1>
      {error && <div className="banner error">{error}</div>}
      <p className="muted">{t('quick.intro')}</p>

      <div className="card">
        <h2>{t('quick.program')}</h2>
        {schedules.length === 0 ? (
          <p className="muted">{t('quick.noPrograms')}</p>
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
              {t('quick.startBtn')}
            </button>
          </div>
        )}
        <p className="muted small">{t('quick.adjustHint')}</p>
      </div>

      <div className="card">
        <h2>{t('quick.custom')}</h2>
        {enabledZones.length === 0 ? (
          <p className="muted">{t('quick.noActiveZones')}</p>
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
                {t('quick.startBtn')}
              </button>
            </div>
            <p className="muted small">{t('quick.rawHint')}</p>
          </>
        )}
      </div>
    </>
  )
}

import { useState } from 'react'
import { Zone, api } from '../api'
import { useToast } from '../components'
import { t } from '../i18n'
import { useLiveState } from '../live'
import { usePoll } from '../util'

export default function Zones() {
  // Zone configs poll slowly; the live on/off state comes via SSE.
  const [data, refresh, error] = usePoll(() => api.zones(), 10000)
  const { state } = useLiveState()
  const toast = useToast()
  // Edits are stored per zone id and fall back to the server state, so no
  // effect is needed to seed local copies.
  const [edit, setEdit] = useState<Record<number, Zone>>({})

  const change = (z: Zone, patch: Partial<Zone>) =>
    setEdit((prev) => ({ ...prev, [z.id]: { ...(prev[z.id] ?? z), ...patch } }))

  const save = async (id: number) => {
    const z = edit[id]
    if (!z) return
    try {
      await api.updateZone(id, { name: z.name, enabled: z.enabled, pump: z.pump })
      setEdit((prev) => {
        const next = { ...prev }
        delete next[id]
        return next
      })
      refresh()
      toast(t('zones.saved'))
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const manual = async (id: number, on: boolean) => {
    try {
      await api.manual(id, on)
      toast(on ? t('zones.started') : t('zones.stopped'))
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const dirty = (z: Zone) => {
    const orig = data?.zones.find((o) => o.id === z.id)
    return !!orig && (orig.name !== z.name || orig.enabled !== z.enabled || orig.pump !== z.pump)
  }

  return (
    <>
      <h1>{t('zones.title')}</h1>
      {error && <div className="banner error">{t('common.noConnection', { msg: error })}</div>}
      <div className="card">
        {data?.zones.map((z) => {
          const e = edit[z.id] ?? z
          const on = state?.zonesOn?.[z.id] ?? z.on
          return (
            <div className="zone-row" key={z.id}>
              <span className={`pill ${on ? 'run' : z.enabled ? 'on' : 'off'}`}>
                <span className="dot" />
                {on ? t('zones.running') : z.enabled ? t('zones.active') : t('zones.off')}
              </span>
              <div className="name">
                <input
                  type="text"
                  value={e.name}
                  aria-label={t('zones.nameAria', { n: z.id + 1 })}
                  onChange={(ev) => change(z, { name: ev.target.value })}
                />
              </div>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={e.enabled}
                  onChange={(ev) => change(z, { enabled: ev.target.checked })}
                />
                {t('zones.active')}
              </label>
              <label className="checkbox" title={t('zones.pumpTitle')}>
                <input
                  type="checkbox"
                  checked={e.pump}
                  onChange={(ev) => change(z, { pump: ev.target.checked })}
                />
                {t('zones.pump')}
              </label>
              <button disabled={!dirty(e)} onClick={() => save(z.id)}>
                {t('common.save')}
              </button>
              {on ? (
                <button className="danger" onClick={() => manual(z.id, false)}>
                  {t('common.stop')}
                </button>
              ) : (
                <button
                  className="primary"
                  disabled={!z.enabled}
                  onClick={() => manual(z.id, true)}
                >
                  {t('common.start')}
                </button>
              )}
            </div>
          )
        })}
      </div>
      <p className="muted small">{t('zones.hint')}</p>
    </>
  )
}

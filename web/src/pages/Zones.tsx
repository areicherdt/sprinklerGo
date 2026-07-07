import { useState } from 'react'
import { api, Zone } from '../api'
import { usePoll } from '../util'

export default function Zones() {
  const [data, refresh, error] = usePoll(() => api.zones(), 3000)
  // Edits are stored per zone id and fall back to the server state, so no
  // effect is needed to seed local copies.
  const [edit, setEdit] = useState<Record<number, Zone>>({})
  const [saveError, setSaveError] = useState<string | null>(null)

  const change = (z: Zone, patch: Partial<Zone>) =>
    setEdit((prev) => ({ ...prev, [z.id]: { ...(prev[z.id] ?? z), ...patch } }))

  const save = async (id: number) => {
    const z = edit[id]
    if (!z) return
    try {
      setSaveError(null)
      await api.updateZone(id, { name: z.name, enabled: z.enabled, pump: z.pump })
      setEdit((prev) => {
        const next = { ...prev }
        delete next[id]
        return next
      })
      refresh()
    } catch (e) {
      setSaveError((e as Error).message)
    }
  }

  const manual = async (id: number, on: boolean) => {
    try {
      setSaveError(null)
      await api.manual(id, on)
      refresh()
    } catch (e) {
      setSaveError((e as Error).message)
    }
  }

  const dirty = (z: Zone) => {
    const orig = data?.zones.find((o) => o.id === z.id)
    return !!orig && (orig.name !== z.name || orig.enabled !== z.enabled || orig.pump !== z.pump)
  }

  return (
    <>
      <h1>Zonen</h1>
      {error && <div className="banner error">Keine Verbindung zum Server: {error}</div>}
      {saveError && <div className="banner error">{saveError}</div>}
      <div className="card">
        {data?.zones.map((z) => {
          const e = edit[z.id] ?? z
          return (
            <div className="zone-row" key={z.id}>
              <span className={`pill ${z.on ? 'run' : z.enabled ? 'on' : 'off'}`}>
                <span className="dot" />
                {z.on ? 'läuft' : z.enabled ? 'aktiv' : 'aus'}
              </span>
              <div className="name">
                <input
                  type="text"
                  value={e.name}
                  aria-label={`Name Zone ${z.id + 1}`}
                  onChange={(ev) => change(z, { name: ev.target.value })}
                />
              </div>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={e.enabled}
                  onChange={(ev) => change(z, { enabled: ev.target.checked })}
                />
                aktiv
              </label>
              <label className="checkbox" title="Pumpe/Hauptventil mitschalten">
                <input
                  type="checkbox"
                  checked={e.pump}
                  onChange={(ev) => change(z, { pump: ev.target.checked })}
                />
                Pumpe
              </label>
              <button disabled={!dirty(e)} onClick={() => save(z.id)}>
                Speichern
              </button>
              {z.on ? (
                <button className="danger" onClick={() => manual(z.id, false)}>
                  Stopp
                </button>
              ) : (
                <button
                  className="primary"
                  disabled={!z.enabled}
                  onClick={() => manual(z.id, true)}
                >
                  Start
                </button>
              )}
            </div>
          )
        })}
      </div>
      <p className="muted small">
        Manueller Start läuft ohne Zeitbegrenzung, bis die Zone gestoppt wird. Es läuft immer nur
        eine Zone gleichzeitig; „Pumpe" schaltet das Hauptventil (Ausgang 0) mit.
      </p>
    </>
  )
}

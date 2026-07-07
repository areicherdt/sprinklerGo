import { useEffect, useState } from 'react'
import { api, Settings as SettingsT, WeatherCheck } from '../api'
import { useToast } from '../components'
import { fmtSeconds } from '../util'

export default function Settings() {
  const [form, setForm] = useState<SettingsT | null>(null)
  const [pins, setPins] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [warn, setWarn] = useState<string | null>(null)
  const [check, setCheck] = useState<WeatherCheck | null>(null)
  const toast = useToast()

  useEffect(() => {
    api
      .settings()
      .then((s) => {
        setForm(s)
        setPins(s.gpioPins.join(', '))
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  if (!form)
    return error ? <div className="banner error">{error}</div> : <p className="muted">Lade…</p>

  const patch = (p: Partial<SettingsT>) => setForm({ ...form, ...p })

  const save = async () => {
    setError(null)
    setNotice(null)
    setWarn(null)
    const gpioPins = pins.split(',').map((s) => Number(s.trim()))
    if (gpioPins.some((n) => Number.isNaN(n) || n < 0)) {
      setError('GPIO-Pins: bitte 16 Zahlen, durch Kommas getrennt.')
      return
    }
    try {
      const res = await api.saveSettings({ ...form, gpioPins })
      if (res.outputError)
        setWarn(
          `Gespeichert, aber der Ausgang konnte nicht initialisiert werden: ${res.outputError}`,
        )
      else if (res.restartRequired)
        setNotice('Gespeichert. Der neue Web-Port gilt nach einem Neustart des Dienstes.')
      else toast('Einstellungen gespeichert.')
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const runCheck = async () => {
    setCheck(null)
    try {
      setCheck(await api.weatherCheck())
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <>
      <h1>Einstellungen</h1>
      {error && <div className="banner error">{error}</div>}
      {notice && <div className="banner info">{notice}</div>}
      {warn && <div className="banner warn">{warn}</div>}

      <div className="card">
        <h2>Bewässerung</h2>
        <label className="field">
          <span>Saisonale Anpassung: {form.seasonalAdjust} %</span>
          <input
            type="number"
            min={0}
            max={200}
            value={form.seasonalAdjust}
            onChange={(e) => patch({ seasonalAdjust: Number(e.target.value) || 0 })}
          />
        </label>
        <p className="muted small">
          Skaliert alle Programmlaufzeiten (100 % = keine Anpassung). Wird mit der Wetter-Anpassung
          multipliziert.
        </p>
      </div>

      <div className="card">
        <h2>Ausgänge</h2>
        <label className="field">
          <span>Ausgabetyp</span>
          <select
            value={form.outputType}
            onChange={(e) => patch({ outputType: e.target.value as SettingsT['outputType'] })}
          >
            <option value="none">Keiner (Testbetrieb)</option>
            <option value="script">Externes Skript</option>
            <option value="gpio+">GPIO direkt (aktiv high)</option>
            <option value="gpio-">GPIO direkt (aktiv low)</option>
          </select>
        </label>
        {form.outputType === 'script' && (
          <label className="field">
            <span>Skript-Pfad (aufgerufen als: Pfad &lt;Ausgang&gt; &lt;0|1&gt;)</span>
            <input
              type="text"
              value={form.scriptPath}
              style={{ width: '100%', maxWidth: 420 }}
              onChange={(e) => patch({ scriptPath: e.target.value })}
            />
          </label>
        )}
        {(form.outputType === 'gpio+' || form.outputType === 'gpio-') && (
          <label className="field">
            <span>GPIO-Pins (BCM, 16 Werte: Pumpe + 15 Zonen)</span>
            <input
              type="text"
              value={pins}
              style={{ width: '100%', maxWidth: 420 }}
              onChange={(e) => setPins(e.target.value)}
            />
          </label>
        )}
      </div>

      <div className="card">
        <h2>Wetter</h2>
        <label className="field">
          <span>Anbieter</span>
          <select
            value={form.weatherProvider}
            onChange={(e) => patch({ weatherProvider: e.target.value })}
          >
            <option value="none">Keiner (keine Anpassung)</option>
            <option value="openmeteo">Open-Meteo (kostenlos, ohne API-Key)</option>
          </select>
        </label>
        {form.weatherProvider !== 'none' && (
          <label className="field">
            <span>Standort als „Breitengrad,Längengrad&quot; (z. B. „52.52,13.40&quot;)</span>
            <input
              type="text"
              value={form.location}
              style={{ maxWidth: 300 }}
              onChange={(e) => patch({ location: e.target.value })}
            />
          </label>
        )}
        <button onClick={runCheck}>Wetter-Diagnose ausführen</button>
        {check && (
          <div
            className={`banner ${!check.noProvider && !check.vals.valid ? 'warn' : 'info'}`}
            style={{ marginTop: 12 }}
          >
            {check.noProvider ? (
              <>Kein Wetter-Anbieter konfiguriert — Skalierung bleibt bei 100 %.</>
            ) : check.vals.valid ? (
              <>
                Anbieter „{check.provider}": OK · Skalierung: <strong>{check.scale} %</strong> ·
                Gestern Ø {Math.round(((check.vals.meanTempF - 32) * 5) / 9)} °C, Feuchte{' '}
                {check.vals.minHumidity}–{check.vals.maxHumidity} %, Regen{' '}
                {(check.vals.precipYesterday * 0.254).toFixed(1)} mm · Heute Regen{' '}
                {(check.vals.precipToday * 0.254).toFixed(1)} mm
              </>
            ) : (
              <>
                Anbieter „{check.provider}" liefert keine Daten:{' '}
                {check.vals.error ?? 'unbekannter Fehler'}
              </>
            )}
          </div>
        )}
        <p className="muted small">
          Die Wetter-Anpassung skaliert Programme mit aktivierter „Wetter-Anpassung" auf 0–200 %
          anhand von Temperatur, Luftfeuchte und Regen des Vortags.
        </p>
      </div>

      <div className="card">
        <h2>System</h2>
        <label className="field">
          <span>Web-Port</span>
          <input
            type="number"
            min={1}
            max={65535}
            value={form.webPort}
            onChange={(e) => patch({ webPort: Number(e.target.value) || 8080 })}
          />
        </label>
        <label className="field">
          <span>Verlauf aufbewahren (Monate, 0 = unbegrenzt)</span>
          <input
            type="number"
            min={0}
            max={120}
            value={form.logRetentionMonths}
            onChange={(e) => patch({ logRetentionMonths: Number(e.target.value) || 0 })}
          />
        </label>
        <label className="field">
          <span>Timer für manuelle Läufe (Minuten, 0 = unbegrenzt)</span>
          <input
            type="number"
            min={0}
            max={1440}
            value={form.manualTimerMinutes}
            onChange={(e) => patch({ manualTimerMinutes: Number(e.target.value) || 0 })}
          />
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={form.clock24h}
            onChange={(e) => patch({ clock24h: e.target.checked })}
          />
          24-Stunden-Format
        </label>
        <p className="muted small" style={{ marginTop: 8 }}>
          Beispiel Restlaufzeit-Format: {fmtSeconds(4980)}
        </p>
      </div>

      <button className="primary" onClick={save}>
        Speichern
      </button>
    </>
  )
}

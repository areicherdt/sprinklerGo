import { useEffect, useRef, useState } from 'react'
import { AuthStatus, api, Settings as SettingsT, WeatherCheck } from '../api'
import { useToast } from '../components'
import { MONTHS, fmtSeconds } from '../util'

function SecurityCard() {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [tokenName, setTokenName] = useState('')
  const [newToken, setNewToken] = useState<string | null>(null)
  const toast = useToast()

  const refresh = () =>
    api
      .auth()
      .then(setStatus)
      .catch(() => {})
  useEffect(() => {
    refresh()
  }, [])

  if (!status) return null

  const toggle = async (enabled: boolean) => {
    try {
      await api.setAuthEnabled(enabled)
      toast(enabled ? 'Anmeldung aktiviert.' : 'Anmeldung deaktiviert.')
      refresh()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const savePassword = async () => {
    try {
      await api.changePassword(currentPw, newPw)
      toast('Passwort gespeichert.')
      setCurrentPw('')
      setNewPw('')
      refresh()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const createToken = async () => {
    try {
      const res = await api.createToken(tokenName.trim())
      setNewToken(res.token)
      setTokenName('')
      refresh()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const removeToken = async (name: string) => {
    try {
      await api.deleteToken(name)
      toast(`Token „${name}" widerrufen.`)
      refresh()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  return (
    <div className="card">
      <h2>Sicherheit</h2>
      <div className="row spread" style={{ marginBottom: 10 }}>
        <span>Anmeldung erforderlich</span>
        <label className="switch">
          <input
            type="checkbox"
            checked={status.enabled}
            disabled={!status.hasPassword && !status.enabled}
            onChange={(e) => toggle(e.target.checked)}
          />
          <span className="slider" />
        </label>
      </div>
      {!status.hasPassword && (
        <p className="muted small">
          Zuerst ein Passwort setzen, dann lässt sich die Anmeldung aktivieren.
        </p>
      )}
      <div className="row">
        {status.hasPassword && (
          <label className="field" style={{ flex: 1, minWidth: 160 }}>
            <span>Aktuelles Passwort</span>
            <input
              type="password"
              value={currentPw}
              style={{ width: '100%' }}
              onChange={(e) => setCurrentPw(e.target.value)}
            />
          </label>
        )}
        <label className="field" style={{ flex: 1, minWidth: 160 }}>
          <span>Neues Passwort (min. 6 Zeichen)</span>
          <input
            type="password"
            value={newPw}
            style={{ width: '100%' }}
            onChange={(e) => setNewPw(e.target.value)}
          />
        </label>
      </div>
      <div className="row" style={{ marginBottom: 14 }}>
        <button disabled={newPw.length < 6} onClick={savePassword}>
          Passwort speichern
        </button>
        {status.enabled && (
          <button onClick={() => api.logout().then(() => window.location.reload())}>
            Abmelden
          </button>
        )}
      </div>

      <h2 style={{ fontSize: 14 }}>API-Tokens für Automatisierung</h2>
      {(status.tokens ?? []).map((t) => (
        <div className="zone-row" key={t.name}>
          <span className="name">{t.name}</span>
          <span className="muted small">
            seit {new Date(t.createdAt * 1000).toLocaleDateString('de-DE')}
          </span>
          <button className="danger" onClick={() => removeToken(t.name)}>
            Widerrufen
          </button>
        </div>
      ))}
      <div className="row" style={{ marginTop: 8 }}>
        <input
          type="text"
          placeholder="Token-Name (z. B. homeassistant)"
          value={tokenName}
          onChange={(e) => setTokenName(e.target.value)}
        />
        <button disabled={tokenName.trim() === ''} onClick={createToken}>
          Token erzeugen
        </button>
      </div>
      {newToken && (
        <div className="banner info" style={{ marginTop: 10 }}>
          Neues Token (wird nur einmal angezeigt): <code>{newToken}</code>
          <br />
          Verwendung: Header <code>Authorization: Bearer &lt;Token&gt;</code>
        </div>
      )}
      <p className="muted small">
        Bei aktivierter Anmeldung verlangen alle API-Aufrufe eine Sitzung oder ein Token; die
        Weboberfläche zeigt eine Login-Seite.
      </p>
    </div>
  )
}

export default function Settings() {
  const [form, setForm] = useState<SettingsT | null>(null)
  const [pins, setPins] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [warn, setWarn] = useState<string | null>(null)
  const [check, setCheck] = useState<WeatherCheck | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
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
          <span>Saisonale Anpassung</span>
          <select
            value={form.seasonalMode}
            onChange={(e) => patch({ seasonalMode: e.target.value as SettingsT['seasonalMode'] })}
          >
            <option value="global">Ein globaler Prozentwert</option>
            <option value="monthly">Monatsprofil (12 Werte)</option>
          </select>
        </label>
        {form.seasonalMode === 'global' ? (
          <label className="field">
            <span>Anpassung: {form.seasonalAdjust} %</span>
            <input
              type="number"
              min={0}
              max={200}
              value={form.seasonalAdjust}
              onChange={(e) => patch({ seasonalAdjust: Number(e.target.value) || 0 })}
            />
          </label>
        ) : (
          <div className="month-grid">
            {MONTHS.map((m, i) => (
              <label className="field" key={m}>
                <span>{m}</span>
                <input
                  type="number"
                  min={0}
                  max={200}
                  value={form.seasonalMonthly[i] ?? 100}
                  onChange={(e) => {
                    const monthly = [...form.seasonalMonthly]
                    while (monthly.length < 12) monthly.push(100)
                    monthly[i] = Math.max(0, Math.min(200, Number(e.target.value) || 0))
                    patch({ seasonalMonthly: monthly })
                  }}
                />
              </label>
            ))}
          </div>
        )}
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
        <div className="row" style={{ gap: 24 }}>
          <label className="field">
            <span>Pumpen-Vorlauf (Sekunden)</span>
            <input
              type="number"
              min={0}
              max={120}
              value={form.pumpPreSeconds}
              onChange={(e) => patch({ pumpPreSeconds: Number(e.target.value) || 0 })}
            />
          </label>
          <label className="field">
            <span>Pumpen-Nachlauf (Sekunden)</span>
            <input
              type="number"
              min={0}
              max={120}
              value={form.pumpPostSeconds}
              onChange={(e) => patch({ pumpPostSeconds: Number(e.target.value) || 0 })}
            />
          </label>
        </div>
        <p className="muted small">
          Vorlauf: Pumpe/Hauptventil startet vor dem Zonenventil; Nachlauf: läuft nach dem Schließen
          nach (0 = gleichzeitig).
        </p>
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
        <h2>Integration</h2>
        <div className="row spread" style={{ marginBottom: 10 }}>
          <span>MQTT (inkl. Home-Assistant-Discovery)</span>
          <label className="switch">
            <input
              type="checkbox"
              checked={form.mqttEnabled}
              onChange={(e) => patch({ mqttEnabled: e.target.checked })}
            />
            <span className="slider" />
          </label>
        </div>
        {form.mqttEnabled && (
          <>
            <label className="field">
              <span>Broker (z. B. „tcp://192.168.1.10:1883&quot;)</span>
              <input
                type="text"
                value={form.mqttBroker}
                style={{ width: '100%', maxWidth: 380 }}
                onChange={(e) => patch({ mqttBroker: e.target.value })}
              />
            </label>
            <div className="row">
              <label className="field" style={{ flex: 1, minWidth: 160 }}>
                <span>Benutzername</span>
                <input
                  type="text"
                  value={form.mqttUsername}
                  style={{ width: '100%' }}
                  onChange={(e) => patch({ mqttUsername: e.target.value })}
                />
              </label>
              <label className="field" style={{ flex: 1, minWidth: 160 }}>
                <span>Passwort</span>
                <input
                  type="password"
                  value={form.mqttPassword}
                  style={{ width: '100%' }}
                  onChange={(e) => patch({ mqttPassword: e.target.value })}
                />
              </label>
            </div>
            <div className="row">
              <label className="field">
                <span>Topic-Präfix</span>
                <input
                  type="text"
                  value={form.mqttTopicPrefix}
                  onChange={(e) => patch({ mqttTopicPrefix: e.target.value })}
                />
              </label>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={form.mqttHADiscovery}
                  onChange={(e) => patch({ mqttHADiscovery: e.target.checked })}
                />
                Home-Assistant-Discovery
              </label>
            </div>
            <p className="muted small">
              Zonen erscheinen in Home Assistant automatisch als Schalter, dazu Automatik,
              Regenpause, Stopp-Taste und Sensoren für aktive Zone und Wetter-Skalierung.
            </p>
          </>
        )}
        <label className="field" style={{ marginTop: 8 }}>
          <span>Webhook-URL für Ereignisse (leer = aus)</span>
          <input
            type="text"
            value={form.webhookUrl}
            placeholder="https://…"
            style={{ width: '100%', maxWidth: 420 }}
            onChange={(e) => patch({ webhookUrl: e.target.value })}
          />
        </label>
        <p className="muted small">
          Sendet JSON-POSTs bei „Lauf gestartet/beendet&quot;, Regenpausen-Übersprüngen sowie
          Ausgangs- und Wetterfehlern — z. B. an ntfy oder Node-RED.
        </p>
      </div>

      <SecurityCard />

      <div className="card">
        <h2>Sicherung</h2>
        <div className="row">
          <a href="/api/backup" download>
            <button>Konfiguration exportieren</button>
          </a>
          <input
            ref={fileRef}
            type="file"
            accept="application/json,.json"
            style={{ display: 'none' }}
            onChange={async (e) => {
              const file = e.target.files?.[0]
              e.target.value = ''
              if (!file) return
              try {
                const res = await api.restore(await file.text())
                toast(
                  res.restartRequired
                    ? 'Wiederhergestellt — neuer Web-Port gilt nach Neustart.'
                    : 'Konfiguration wiederhergestellt.',
                )
                const s = await api.settings()
                setForm(s)
                setPins(s.gpioPins.join(', '))
              } catch (err) {
                toast((err as Error).message, 'error')
              }
            }}
          />
          <button type="button" onClick={() => fileRef.current?.click()}>
            Sicherung importieren…
          </button>
        </div>
        <p className="muted small">
          Der Export enthält Zonen, Programme und alle Einstellungen (inkl. Zugangsdaten). Beim
          Import wird eine ältere Sicherung automatisch migriert; laufende Bewässerung stoppt.
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

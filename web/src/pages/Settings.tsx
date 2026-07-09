import { useEffect, useRef, useState } from 'react'
import { AuthStatus, api, Settings as SettingsT, WeatherCheck } from '../api'
import { useToast } from '../components'
import { getLanguage, locale, months, t } from '../i18n'
import { fmtSeconds } from '../util'

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
      toast(enabled ? t('set.authOn') : t('set.authOff'))
      refresh()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  const savePassword = async () => {
    try {
      await api.changePassword(currentPw, newPw)
      toast(t('set.pwSaved'))
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
      toast(t('set.tokenRevoked', { name }))
      refresh()
    } catch (e) {
      toast((e as Error).message, 'error')
    }
  }

  return (
    <div className="card">
      <h2>{t('set.security')}</h2>
      <div className="row spread" style={{ marginBottom: 10 }}>
        <span>{t('set.authRequired')}</span>
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
      {!status.hasPassword && <p className="muted small">{t('set.authNeedPassword')}</p>}
      <div className="row">
        {status.hasPassword && (
          <label className="field" style={{ flex: 1, minWidth: 160 }}>
            <span>{t('set.currentPw')}</span>
            <input
              type="password"
              value={currentPw}
              style={{ width: '100%' }}
              onChange={(e) => setCurrentPw(e.target.value)}
            />
          </label>
        )}
        <label className="field" style={{ flex: 1, minWidth: 160 }}>
          <span>{t('set.newPw')}</span>
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
          {t('set.savePw')}
        </button>
        {status.enabled && (
          <button onClick={() => api.logout().then(() => window.location.reload())}>
            {t('set.signOut')}
          </button>
        )}
      </div>

      <h2 style={{ fontSize: 14 }}>{t('set.tokens')}</h2>
      {(status.tokens ?? []).map((tok) => (
        <div className="zone-row" key={tok.name}>
          <span className="name">{tok.name}</span>
          <span className="muted small">
            {t('set.tokenSince', {
              d: new Date(tok.createdAt * 1000).toLocaleDateString(locale()),
            })}
          </span>
          <button className="danger" onClick={() => removeToken(tok.name)}>
            {t('set.revoke')}
          </button>
        </div>
      ))}
      <div className="row" style={{ marginTop: 8 }}>
        <input
          type="text"
          placeholder={t('set.tokenPlaceholder')}
          value={tokenName}
          onChange={(e) => setTokenName(e.target.value)}
        />
        <button disabled={tokenName.trim() === ''} onClick={createToken}>
          {t('set.createToken')}
        </button>
      </div>
      {newToken && (
        <div className="banner info" style={{ marginTop: 10 }}>
          {t('set.newTokenNote')} <code>{newToken}</code>
          <br />
          {t('set.tokenUsage')} <code>Authorization: Bearer &lt;Token&gt;</code>
        </div>
      )}
      <p className="muted small">{t('set.securityHint')}</p>
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
  const [movedTo, setMovedTo] = useState<string | null>(null)
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
    return error ? (
      <div className="banner error">{error}</div>
    ) : (
      <p className="muted">{t('common.loading')}</p>
    )

  const patch = (p: Partial<SettingsT>) => setForm({ ...form, ...p })

  const save = async () => {
    setError(null)
    setNotice(null)
    setWarn(null)
    const gpioPins = pins.split(',').map((s) => Number(s.trim()))
    if (gpioPins.some((n) => Number.isNaN(n) || n < 0)) {
      setError(t('set.gpioPinsError'))
      return
    }
    try {
      const res = await api.saveSettings({ ...form, gpioPins })
      // A language change only takes effect on a full render — reload so the
      // whole app picks up the new dictionary.
      if (form.language !== getLanguage()) {
        window.location.reload()
        return
      }
      if (res.outputError) setWarn(t('set.savedOutputWarn', { err: res.outputError }))
      else if (res.portChanged && res.webPort) {
        // The server already moved; this origin goes dark in a few seconds.
        const target = `${window.location.protocol}//${window.location.hostname}:${res.webPort}/settings`
        setMovedTo(target)
      } else if (res.restartRequired) setNotice(t('set.savedRestart'))
      else toast(t('set.saved'))
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
      <h1>{t('set.title')}</h1>
      {error && <div className="banner error">{error}</div>}
      {notice && <div className="banner info">{notice}</div>}
      {warn && <div className="banner warn">{warn}</div>}
      {movedTo && (
        <div className="banner info">
          {t('set.portMoved')} <a href={movedTo}>{movedTo}</a>
        </div>
      )}

      <div className="card">
        <h2>{t('set.watering')}</h2>
        <label className="field">
          <span>{t('set.seasonal')}</span>
          <select
            value={form.seasonalMode}
            onChange={(e) => patch({ seasonalMode: e.target.value as SettingsT['seasonalMode'] })}
          >
            <option value="global">{t('set.seasonalGlobal')}</option>
            <option value="monthly">{t('set.seasonalMonthly')}</option>
          </select>
        </label>
        {form.seasonalMode === 'global' ? (
          <label className="field">
            <span>{t('set.adjustment', { n: form.seasonalAdjust })}</span>
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
            {months().map((m, i) => (
              <label className="field" key={i}>
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
        <p className="muted small">{t('set.seasonalHint')}</p>
      </div>

      <div className="card">
        <h2>{t('set.outputs')}</h2>
        <label className="field">
          <span>{t('set.outputType')}</span>
          <select
            value={form.outputType}
            onChange={(e) => patch({ outputType: e.target.value as SettingsT['outputType'] })}
          >
            <option value="none">{t('set.outputNone')}</option>
            <option value="script">{t('set.outputScript')}</option>
            <option value="gpio+">{t('set.outputGpioHigh')}</option>
            <option value="gpio-">{t('set.outputGpioLow')}</option>
            <option value="greeniq">{t('set.outputGreenIQ')}</option>
          </select>
        </label>
        {form.outputType === 'greeniq' && <p className="muted small">{t('set.greeniqHint')}</p>}
        {form.outputType === 'script' && (
          <label className="field">
            <span>{t('set.scriptPath')}</span>
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
            <span>{t('set.gpioPins')}</span>
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
            <span>{t('set.pumpPre')}</span>
            <input
              type="number"
              min={0}
              max={120}
              value={form.pumpPreSeconds}
              onChange={(e) => patch({ pumpPreSeconds: Number(e.target.value) || 0 })}
            />
          </label>
          <label className="field">
            <span>{t('set.pumpPost')}</span>
            <input
              type="number"
              min={0}
              max={120}
              value={form.pumpPostSeconds}
              onChange={(e) => patch({ pumpPostSeconds: Number(e.target.value) || 0 })}
            />
          </label>
        </div>
        <p className="muted small">{t('set.pumpHint')}</p>
      </div>

      <div className="card">
        <h2>{t('set.weather')}</h2>
        <label className="field">
          <span>{t('set.provider')}</span>
          <select
            value={form.weatherProvider}
            onChange={(e) => patch({ weatherProvider: e.target.value })}
          >
            <option value="none">{t('set.providerNone')}</option>
            <option value="openmeteo">{t('set.providerOpenMeteo')}</option>
            <option value="openweather">{t('set.providerOpenWeather')}</option>
          </select>
        </label>
        {form.weatherProvider !== 'none' && (
          <label className="field">
            <span>{t('set.location')}</span>
            <input
              type="text"
              value={form.location}
              style={{ maxWidth: 300 }}
              onChange={(e) => patch({ location: e.target.value })}
            />
          </label>
        )}
        {form.weatherProvider === 'openweather' && (
          <label className="field">
            <span>{t('set.apiKey')}</span>
            <input
              type="password"
              value={form.apiSecret}
              style={{ width: '100%', maxWidth: 380 }}
              onChange={(e) => patch({ apiSecret: e.target.value })}
            />
          </label>
        )}
        <button onClick={runCheck}>{t('set.runDiag')}</button>
        {check && (
          <div
            className={`banner ${!check.noProvider && !check.vals.valid ? 'warn' : 'info'}`}
            style={{ marginTop: 12 }}
          >
            {check.noProvider ? (
              t('set.diagNoProvider')
            ) : check.vals.valid ? (
              <>
                {t('set.diagResult', {
                  provider: check.provider,
                  scale: check.scale,
                  temp: Math.round(((check.vals.meanTempF - 32) * 5) / 9),
                  hmin: check.vals.minHumidity,
                  hmax: check.vals.maxHumidity,
                  rain: (check.vals.precipYesterday * 0.254).toFixed(1),
                  rainToday: (check.vals.precipToday * 0.254).toFixed(1),
                })}
              </>
            ) : (
              t('set.diagError', {
                provider: check.provider,
                err: check.vals.error ?? t('set.diagUnknown'),
              })
            )}
          </div>
        )}
        <p className="muted small">{t('set.weatherHint')}</p>
      </div>

      <div className="card">
        <h2>{t('set.integration')}</h2>
        <div className="row spread" style={{ marginBottom: 10 }}>
          <span>{t('set.mqtt')}</span>
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
              <span>{t('set.broker')}</span>
              <input
                type="text"
                value={form.mqttBroker}
                style={{ width: '100%', maxWidth: 380 }}
                onChange={(e) => patch({ mqttBroker: e.target.value })}
              />
            </label>
            <div className="row">
              <label className="field" style={{ flex: 1, minWidth: 160 }}>
                <span>{t('set.username')}</span>
                <input
                  type="text"
                  value={form.mqttUsername}
                  style={{ width: '100%' }}
                  onChange={(e) => patch({ mqttUsername: e.target.value })}
                />
              </label>
              <label className="field" style={{ flex: 1, minWidth: 160 }}>
                <span>{t('set.passwordField')}</span>
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
                <span>{t('set.topicPrefix')}</span>
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
                {t('set.haDiscovery')}
              </label>
            </div>
            <p className="muted small">{t('set.mqttHint')}</p>
          </>
        )}
        <label className="field" style={{ marginTop: 8 }}>
          <span>{t('set.webhook')}</span>
          <input
            type="text"
            value={form.webhookUrl}
            placeholder="https://…"
            style={{ width: '100%', maxWidth: 420 }}
            onChange={(e) => patch({ webhookUrl: e.target.value })}
          />
        </label>
        <p className="muted small">{t('set.webhookHint')}</p>
      </div>

      <SecurityCard />

      <div className="card">
        <h2>{t('set.backup')}</h2>
        <div className="row">
          <a href="/api/backup" download>
            <button>{t('set.export')}</button>
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
                toast(res.restartRequired ? t('set.restoredRestart') : t('set.restored'))
                const s = await api.settings()
                setForm(s)
                setPins(s.gpioPins.join(', '))
              } catch (err) {
                toast((err as Error).message, 'error')
              }
            }}
          />
          <button type="button" onClick={() => fileRef.current?.click()}>
            {t('set.import')}
          </button>
        </div>
        <p className="muted small">{t('set.backupHint')}</p>
      </div>

      <div className="card">
        <h2>{t('set.system')}</h2>
        <label className="field">
          <span>{t('set.webPort')}</span>
          <input
            type="number"
            min={1}
            max={65535}
            value={form.webPort}
            onChange={(e) => patch({ webPort: Number(e.target.value) || 8080 })}
          />
        </label>
        <label className="field">
          <span>{t('set.retention')}</span>
          <input
            type="number"
            min={0}
            max={120}
            value={form.logRetentionMonths}
            onChange={(e) => patch({ logRetentionMonths: Number(e.target.value) || 0 })}
          />
        </label>
        <label className="field">
          <span>{t('set.manualTimer')}</span>
          <input
            type="number"
            min={0}
            max={1440}
            value={form.manualTimerMinutes}
            onChange={(e) => patch({ manualTimerMinutes: Number(e.target.value) || 0 })}
          />
        </label>
        <label className="field">
          <span>{t('set.language')}</span>
          <select
            value={form.language}
            onChange={(e) => patch({ language: e.target.value as SettingsT['language'] })}
          >
            <option value="de">Deutsch</option>
            <option value="en">English</option>
          </select>
        </label>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={form.clock24h}
            onChange={(e) => patch({ clock24h: e.target.checked })}
          />
          {t('set.clock24')}
        </label>
        <p className="muted small" style={{ marginTop: 8, marginBottom: 12 }}>
          {t('set.exampleFmt', { t: fmtSeconds(4980) })}
        </p>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={form.metricsEnabled}
            onChange={(e) => patch({ metricsEnabled: e.target.checked })}
          />
          {t('set.metrics')}
        </label>
        <p className="muted small" style={{ marginTop: 8 }}>
          {t('set.metricsHint')}
        </p>
      </div>

      <button className="primary" onClick={save}>
        {t('common.save')}
      </button>
    </>
  )
}

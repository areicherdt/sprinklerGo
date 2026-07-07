import { FormEvent, useState } from 'react'
import { api } from '../api'

export default function Login({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api.login(password)
      onSuccess()
    } catch {
      setError('Falsches Passwort.')
      setPassword('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main style={{ maxWidth: 380, marginTop: '15vh' }}>
      <div className="card">
        <div className="brand" style={{ paddingBottom: 4 }}>
          <img src="/sprinkler.svg" alt="" />
          sprinklerGo
        </div>
        <p className="muted">Anmeldung erforderlich.</p>
        {error && <div className="banner error">{error}</div>}
        <form onSubmit={submit}>
          <label className="field">
            <span>Passwort</span>
            <input
              type="password"
              value={password}
              autoFocus
              style={{ width: '100%' }}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
          <button className="primary" type="submit" disabled={busy || password === ''}>
            Anmelden
          </button>
        </form>
      </div>
    </main>
  )
}

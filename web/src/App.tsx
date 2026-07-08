import { useEffect, useState } from 'react'
import { NavLink, Route, Routes } from 'react-router-dom'
import { AuthStatus, api } from './api'
import { ToastProvider } from './components'
import { MsgKey, setLanguage, t } from './i18n'
import Dashboard from './pages/Dashboard'
import Login from './pages/Login'
import Zones from './pages/Zones'
import Schedules from './pages/Schedules'
import ScheduleEdit from './pages/ScheduleEdit'
import QuickRun from './pages/QuickRun'
import History from './pages/History'
import Settings from './pages/Settings'

const NAV: { to: string; label: MsgKey }[] = [
  { to: '/', label: 'nav.overview' },
  { to: '/zones', label: 'nav.zones' },
  { to: '/schedules', label: 'nav.schedules' },
  { to: '/quickrun', label: 'nav.quickrun' },
  { to: '/history', label: 'nav.history' },
  { to: '/settings', label: 'nav.settings' },
]

export default function App() {
  const [auth, setAuth] = useState<AuthStatus | null>(null)

  useEffect(() => {
    const check = () =>
      api
        .auth()
        .then((a) => {
          // Die Sprache kommt vor dem ersten Render über die offene
          // Status-Probe, damit auch die Login-Seite übersetzt ist.
          setLanguage(a.language)
          setAuth(a)
        })
        .catch(() =>
          setAuth({ enabled: false, loggedIn: true, hasPassword: false, language: 'de' }),
        )
    check()
    window.addEventListener('sprinklergo:unauthorized', check)
    return () => window.removeEventListener('sprinklergo:unauthorized', check)
  }, [])

  if (auth === null) return null
  if (auth.enabled && !auth.loggedIn) {
    return <Login onSuccess={() => api.auth().then(setAuth)} />
  }

  return (
    <ToastProvider>
      <header className="app-header">
        <div className="inner">
          <div className="brand">
            <img src="/sprinkler.svg" alt="" />
            sprinklerGo
          </div>
          <nav className="main-nav" aria-label="Navigation">
            {NAV.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.to === '/'}
                className={({ isActive }) => (isActive ? 'active' : '')}
              >
                {t(n.label)}
              </NavLink>
            ))}
          </nav>
        </div>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/zones" element={<Zones />} />
          <Route path="/schedules" element={<Schedules />} />
          <Route path="/schedules/:id" element={<ScheduleEdit />} />
          <Route path="/quickrun" element={<QuickRun />} />
          <Route path="/history" element={<History />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </main>
    </ToastProvider>
  )
}

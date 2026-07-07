import { useEffect, useState } from 'react'
import { NavLink, Route, Routes } from 'react-router-dom'
import { AuthStatus, api } from './api'
import { ToastProvider } from './components'
import Dashboard from './pages/Dashboard'
import Login from './pages/Login'
import Zones from './pages/Zones'
import Schedules from './pages/Schedules'
import ScheduleEdit from './pages/ScheduleEdit'
import QuickRun from './pages/QuickRun'
import History from './pages/History'
import Settings from './pages/Settings'

const NAV = [
  { to: '/', label: 'Übersicht' },
  { to: '/zones', label: 'Zonen' },
  { to: '/schedules', label: 'Programme' },
  { to: '/quickrun', label: 'Schnellstart' },
  { to: '/history', label: 'Verlauf' },
  { to: '/settings', label: 'Einstellungen' },
]

export default function App() {
  const [auth, setAuth] = useState<AuthStatus | null>(null)

  useEffect(() => {
    const check = () =>
      api
        .auth()
        .then(setAuth)
        .catch(() => setAuth({ enabled: false, loggedIn: true, hasPassword: false }))
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
          <nav className="main-nav">
            {NAV.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.to === '/'}
                className={({ isActive }) => (isActive ? 'active' : '')}
              >
                {n.label}
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

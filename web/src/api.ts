// Typed client for the sprinklerGo REST API.

export interface PlannedStart {
  scheduleId: number
  scheduleName: string
  at: number
  waiting: boolean
}

export interface QueuedZoneRun {
  zoneId: number
  zoneName: string
  start: number
  end: number
  done: boolean
  active: boolean
}

export interface WeatherInfo {
  provider: string
  scale: number
  valid: boolean
  fetchedAt: number
}

export interface StateDTO {
  version: string
  time: number
  schedulerEnabled: boolean
  mode: 'idle' | 'schedule' | 'manual' | 'soaking'
  zoneId: number
  zoneName?: string
  scheduleId: number
  scheduleName?: string
  remainingSeconds: number
  pendingEvents: number
  enabledZones: number
  scheduleCount: number
  rainDelayUntil: number
  clock24h: boolean
  language: string
  zonesOn: boolean[]
  pumpOn: boolean
  planned: PlannedStart[]
  queue: QueuedZoneRun[]
  weather: WeatherInfo
}

export interface Zone {
  id: number
  name: string
  enabled: boolean
  pump: boolean
  on: boolean
}

export interface NextRun {
  date: string
  inDays: number
  times: number[]
}

export interface Schedule {
  id: number
  name: string
  enabled: boolean
  kind: 'weekly' | 'interval'
  days: boolean[]
  interval: number
  restriction: number
  weatherAdjust: boolean
  startTimes: number[]
  durations: number[]
  cycleMaxMinutes: number
  soakMinutes: number
  nextRun: NextRun | null
}

// SchedulePayload is what POST/PUT accept: the schedule without the
// server-computed fields (the API rejects unknown fields).
export type SchedulePayload = Omit<Schedule, 'id' | 'nextRun'>

export interface Settings {
  webPort: number
  outputType: 'none' | 'script' | 'gpio+' | 'gpio-'
  scriptPath: string
  gpioPins: number[]
  seasonalAdjust: number
  seasonalMode: 'global' | 'monthly'
  seasonalMonthly: number[]
  pumpPreSeconds: number
  pumpPostSeconds: number
  weatherProvider: string
  apiKey: string
  apiSecret: string
  location: string
  clock24h: boolean
  language: 'de' | 'en'
  metricsEnabled: boolean
  runSchedules: boolean
  logRetentionMonths: number
  manualTimerMinutes: number
  mqttEnabled: boolean
  mqttBroker: string
  mqttUsername: string
  mqttPassword: string
  mqttTopicPrefix: string
  mqttHADiscovery: boolean
  webhookUrl: string
}

export interface SaveSettingsResult {
  ok: boolean
  restartRequired?: boolean
  outputError?: string
}

export interface WeatherCheck {
  provider: string
  noProvider: boolean
  scale: number
  vals: {
    valid: boolean
    keyNotFound: boolean
    error?: string
    minHumidity: number
    maxHumidity: number
    meanTempF: number
    precipYesterday: number
    precipToday: number
    windMph: number
    uv: number
  }
}

export interface LogEntry {
  start: number
  zoneId: number
  seconds: number
  scheduleId: number
  seasonal: number
  weather: number
}

export interface LogBucket {
  bucket: number
  seconds: number
}

export interface ZoneSeries {
  zoneId: number
  buckets: LogBucket[]
}

export type Grouping = 'none' | 'hour' | 'day' | 'month'

export interface AuthStatus {
  enabled: boolean
  loggedIn: boolean
  hasPassword: boolean
  language: string
  tokens?: { name: string; createdAt: number }[]
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('/api/auth')) {
      // Session expired: let the app shell re-check and show the login.
      window.dispatchEvent(new Event('sprinklergo:unauthorized'))
    }
    throw new Error((data as { error?: string }).error ?? `HTTP ${res.status}`)
  }
  return data as T
}

export const api = {
  state: () => req<StateDTO>('GET', '/api/state'),

  zones: () => req<{ zones: Zone[]; pumpOn: boolean }>('GET', '/api/zones'),
  updateZone: (id: number, z: { name: string; enabled: boolean; pump: boolean }) =>
    req('PUT', `/api/zones/${id}`, z),
  manual: (id: number, on: boolean, minutes?: number) =>
    req('POST', `/api/zones/${id}/manual`, minutes === undefined ? { on } : { on, minutes }),
  rainDelay: (hours: number) => req('PUT', '/api/rain-delay', { hours }),

  schedules: () => req<{ schedules: Schedule[] }>('GET', '/api/schedules'),
  schedule: (id: number) => req<Schedule>('GET', `/api/schedules/${id}`),
  createSchedule: (s: SchedulePayload) => req<{ id: number }>('POST', '/api/schedules', s),
  updateSchedule: (id: number, s: SchedulePayload) => req('PUT', `/api/schedules/${id}`, s),
  deleteSchedule: (id: number) => req('DELETE', `/api/schedules/${id}`),

  quickRunSchedule: (scheduleId: number) => req('POST', '/api/quickrun', { scheduleId }),
  quickRunDurations: (durations: number[]) => req('POST', '/api/quickrun', { durations }),
  stop: () => req('POST', '/api/stop'),
  setRun: (enabled: boolean) => req('PUT', '/api/system/run', { enabled }),

  settings: () => req<Settings>('GET', '/api/settings'),
  saveSettings: (s: Settings) => req<SaveSettingsResult>('PUT', '/api/settings', s),
  restore: (backup: string) =>
    fetch('/api/restore', { method: 'POST', body: backup }).then(async (r) => {
      const data = (await r.json().catch(() => ({}))) as SaveSettingsResult & { error?: string }
      if (!r.ok) throw new Error(data.error ?? `HTTP ${r.status}`)
      return data
    }),
  weatherCheck: () => req<WeatherCheck>('GET', '/api/weather/check'),

  auth: () => req<AuthStatus>('GET', '/api/auth'),
  login: (password: string) => req('POST', '/api/auth/login', { password }),
  logout: () => req('POST', '/api/auth/logout'),
  setAuthEnabled: (enabled: boolean) => req('PUT', '/api/auth', { enabled }),
  changePassword: (current: string, next: string) =>
    req('POST', '/api/auth/password', { current, new: next }),
  createToken: (name: string) => req<{ token: string }>('POST', '/api/auth/tokens', { name }),
  deleteToken: (name: string) => req('DELETE', `/api/auth/tokens/${encodeURIComponent(name)}`),

  logEntries: (start: number, end: number) =>
    req<{ entries: LogEntry[] }>('GET', `/api/logs?group=none&start=${start}&end=${end}`),
  logSeries: (start: number, end: number, group: Grouping) =>
    req<{ series: ZoneSeries[] }>('GET', `/api/logs?group=${group}&start=${start}&end=${end}`),
}

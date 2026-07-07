// Typed client for the sprinklerGo REST API.

export interface StateDTO {
  version: string
  time: number
  schedulerEnabled: boolean
  mode: 'idle' | 'schedule' | 'manual'
  zoneId: number
  zoneName?: string
  scheduleId: number
  scheduleName?: string
  remainingSeconds: number
  pendingEvents: number
  enabledZones: number
  scheduleCount: number
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
  weatherProvider: string
  apiKey: string
  apiSecret: string
  location: string
  clock24h: boolean
  runSchedules: boolean
  logRetentionMonths: number
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

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error((data as { error?: string }).error ?? `HTTP ${res.status}`)
  }
  return data as T
}

export const api = {
  state: () => req<StateDTO>('GET', '/api/state'),

  zones: () => req<{ zones: Zone[]; pumpOn: boolean }>('GET', '/api/zones'),
  updateZone: (id: number, z: { name: string; enabled: boolean; pump: boolean }) =>
    req('PUT', `/api/zones/${id}`, z),
  manual: (id: number, on: boolean) => req('POST', `/api/zones/${id}/manual`, { on }),

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
  weatherCheck: () => req<WeatherCheck>('GET', '/api/weather/check'),

  logEntries: (start: number, end: number) =>
    req<{ entries: LogEntry[] }>('GET', `/api/logs?group=none&start=${start}&end=${end}`),
  logSeries: (start: number, end: number, group: Grouping) =>
    req<{ series: ZoneSeries[] }>('GET', `/api/logs?group=${group}&start=${start}&end=${end}`),
}

import { useEffect, useState } from 'react'
import { locale, t } from './i18n'

// Global clock format, fed from the server state (settings.clock24h) by
// useLiveState. Components re-render on state changes anyway.
let clock24 = true
export function setClockFormat(is24: boolean) {
  clock24 = is24
}
export function isClock24(): boolean {
  return clock24
}

/** minutes since midnight -> "06:30" (or "6:30 AM" in 12h mode) */
export function fmtTime(min: number): string {
  const h = Math.floor(min / 60)
  const m = String(min % 60).padStart(2, '0')
  if (clock24) return `${String(h).padStart(2, '0')}:${m}`
  const suffix = h < 12 ? 'AM' : 'PM'
  const h12 = h % 12 === 0 ? 12 : h % 12
  return `${h12}:${m} ${suffix}`
}

/** unix seconds -> time of day, honoring the clock format */
export function fmtClock(unixSeconds: number): string {
  const d = new Date(unixSeconds * 1000)
  return fmtTime(d.getHours() * 60 + d.getMinutes())
}

/** Civil days since 1970-01-01 of the local date (mirrors model.EpochDays). */
export function epochDays(d: Date): number {
  return Math.floor(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()) / 86400000)
}

export interface RunPattern {
  enabled: boolean
  kind: 'weekly' | 'interval'
  days: boolean[]
  interval: number
  restriction: number
}

/** Client-side port of Schedule.RunsOn for the editor's live preview. */
export function runsOn(s: RunPattern, day: Date): boolean {
  if (!s.enabled) return false
  if (s.kind === 'interval') {
    if (s.interval < 1) return false
    return epochDays(day) % s.interval === 0
  }
  if (s.restriction !== 0 && day.getDate() % 2 !== s.restriction % 2) return false
  return !!s.days[day.getDay()]
}

/** The next `count` run days (with times) within 14 days, for previews. */
export function nextRunPreview(
  s: RunPattern,
  startTimes: number[],
  from: Date,
  count: number,
): { day: Date; inDays: number; times: number[] }[] {
  const out: { day: Date; inDays: number; times: number[] }[] = []
  if (startTimes.length === 0) return out
  const nowMin = from.getHours() * 60 + from.getMinutes()
  for (let d = 0; d <= 14 && out.length < count; d++) {
    const day = new Date(from.getFullYear(), from.getMonth(), from.getDate() + d, 12)
    if (!runsOn(s, day)) continue
    const times = startTimes.filter((t) => d > 0 || t > nowMin).sort((a, b) => a - b)
    if (times.length === 0) continue
    out.push({ day, inDays: d, times })
  }
  return out
}

/** "06:30" -> minutes since midnight, or null */
export function parseTime(v: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(v)
  if (!m) return null
  const h = Number(m[1])
  const min = Number(m[2])
  if (h > 23 || min > 59) return null
  return h * 60 + min
}

export function fmtSeconds(total: number): string {
  if (total < 0) return '—'
  const min = Math.floor(total / 60)
  const s = total % 60
  if (min >= 60) {
    const h = Math.floor(min / 60)
    return `${h} h ${min % 60} min`
  }
  if (min > 0) return s > 0 ? `${min} min ${s} s` : `${min} min`
  return `${s} s`
}

export function fmtDate(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString(locale(), {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: !isClock24(),
  })
}

export function fmtNextRun(nr: { date: string; inDays: number; times: number[] } | null): string {
  if (!nr) return t('label.noRunPlanned')
  const times = nr.times.map(fmtTime).join(', ')
  if (nr.inDays === 0) return `${t('time.today')} ${times}`
  if (nr.inDays === 1) return `${t('time.tomorrow')} ${times}`
  return `${t('time.inDays', { n: nr.inDays, date: nr.date })} ${times}`
}

export function scheduleLabel(id: number, names: Map<number, string>): string {
  if (id === -1) return t('label.manual')
  if (id === 99) return t('label.quickRun')
  return names.get(id) ?? `${t('dash.program')} ${id + 1}`
}

/** Poll a fetcher on an interval; returns latest value and a refresh trigger. */
export function usePoll<T>(
  fetcher: () => Promise<T>,
  intervalMs: number,
): [T | null, () => void, string | null] {
  const [value, setValue] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let active = true
    const load = () =>
      fetcher().then(
        (v) => {
          if (active) {
            setValue(v)
            setError(null)
          }
        },
        (e: Error) => {
          if (active) setError(e.message)
        },
      )
    load()
    const id = setInterval(load, intervalMs)
    return () => {
      active = false
      clearInterval(id)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick, intervalMs])

  return [value, () => setTick((t) => t + 1), error]
}

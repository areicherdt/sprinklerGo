import { useEffect, useState } from 'react'

export const WEEKDAYS = ['So', 'Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa']
export const MONTHS = [
  'Jan',
  'Feb',
  'Mär',
  'Apr',
  'Mai',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Okt',
  'Nov',
  'Dez',
]

/** minutes since midnight -> "06:30" */
export function fmtTime(min: number): string {
  const h = Math.floor(min / 60)
  const m = min % 60
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`
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
  return new Date(unixSeconds * 1000).toLocaleString('de-DE', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function fmtNextRun(nr: { date: string; inDays: number; times: number[] } | null): string {
  if (!nr) return 'kein Lauf geplant'
  const times = nr.times.map(fmtTime).join(', ')
  if (nr.inDays === 0) return `Heute ${times}`
  if (nr.inDays === 1) return `Morgen ${times}`
  return `In ${nr.inDays} Tagen (${nr.date}) ${times}`
}

export function scheduleLabel(id: number, names: Map<number, string>): string {
  if (id === -1) return 'Manuell'
  if (id === 99) return 'Schnellstart'
  return names.get(id) ?? `Programm ${id + 1}`
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

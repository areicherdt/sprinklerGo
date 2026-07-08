import { useEffect, useState } from 'react'
import { StateDTO } from './api'
import { setLanguage } from './i18n'
import { setClockFormat } from './util'

export interface LiveState {
  state: StateDTO | null
  receivedAt: number // Date.now() when the state arrived
  error: string | null
}

/**
 * Live system state: subscribes to the SSE stream (/api/events) and falls
 * back to 3-second polling if the stream is unavailable.
 */
export function useLiveState(): LiveState {
  const [live, setLive] = useState<LiveState>({ state: null, receivedAt: 0, error: null })

  useEffect(() => {
    let es: EventSource | null = null
    let pollTimer: ReturnType<typeof setInterval> | null = null
    let closed = false

    const apply = (s: StateDTO) => {
      if (closed) return
      setClockFormat(s.clock24h)
      setLanguage(s.language)
      setLive({ state: s, receivedAt: Date.now(), error: null })
    }
    const fail = (msg: string) => {
      if (closed) return
      setLive((prev) => ({ ...prev, error: msg }))
    }
    const startPolling = () => {
      if (closed || pollTimer) return
      const load = () =>
        fetch('/api/state')
          .then((r) => r.json())
          .then(apply)
          .catch((e: Error) => fail(e.message))
      load()
      pollTimer = setInterval(load, 3000)
    }

    if (typeof EventSource !== 'undefined') {
      es = new EventSource('/api/events')
      es.addEventListener('state', (ev) => apply(JSON.parse((ev as MessageEvent).data)))
      es.onerror = () => {
        // EventSource retries transient errors itself; fall back to polling
        // only once the connection is permanently closed.
        if (es?.readyState === EventSource.CLOSED) {
          es.close()
          es = null
          startPolling()
        }
      }
    } else {
      startPolling()
    }
    return () => {
      closed = true
      es?.close()
      if (pollTimer) clearInterval(pollTimer)
    }
  }, [])

  return live
}

/** Re-renders once per second; returns the current Date.now(). */
export function useNowSecond(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])
  return now
}

/**
 * Locally ticking remaining seconds: server value minus the time elapsed
 * since it arrived. -1 (unlimited) passes through.
 */
export function ticking(remaining: number, receivedAt: number, now: number): number {
  if (remaining < 0) return remaining
  return Math.max(0, remaining - Math.floor((now - receivedAt) / 1000))
}

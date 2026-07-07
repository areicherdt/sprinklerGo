import { useEffect, useMemo, useRef, useState } from 'react'
import { api, Grouping, LogEntry, Zone, ZoneSeries } from '../api'
import { MONTHS, WEEKDAYS, fmtDate, fmtSeconds, scheduleLabel } from '../util'

const RANGES = [
  { label: '7 Tage', days: 7 },
  { label: '30 Tage', days: 30 },
  { label: '12 Monate', days: 365 },
]

const VIEWS: { label: string; group: Grouping }[] = [
  { label: 'Tabelle', group: 'none' },
  { label: 'Nach Stunde', group: 'hour' },
  { label: 'Nach Wochentag', group: 'day' },
  { label: 'Nach Monat', group: 'month' },
]

function bucketDomain(group: Grouping): number[] {
  if (group === 'hour') return Array.from({ length: 24 }, (_, i) => i)
  if (group === 'day') return Array.from({ length: 7 }, (_, i) => i)
  return Array.from({ length: 12 }, (_, i) => i + 1)
}

function bucketLabel(group: Grouping, b: number): string {
  if (group === 'hour') return `${b} Uhr`
  if (group === 'day') return WEEKDAYS[b]
  return MONTHS[b - 1]
}

function bucketTick(group: Grouping, b: number): string {
  if (group === 'hour') return b % 3 === 0 ? String(b) : ''
  if (group === 'day') return WEEKDAYS[b]
  return MONTHS[b - 1]
}

interface Tip {
  x: number
  y: number
  title: string
  total: number
  perZone: { name: string; seconds: number }[]
}

/** Single-series bar chart: total watering time per bucket, zone breakdown in
 * the tooltip. One hue, no legend (the title names the series). */
function BarChart({
  series,
  group,
  zoneNames,
}: {
  series: ZoneSeries[]
  group: Grouping
  zoneNames: Map<number, string>
}) {
  const [tip, setTip] = useState<Tip | null>(null)
  const wrapRef = useRef<HTMLDivElement>(null)

  const domain = bucketDomain(group)
  const totals = new Map<number, number>()
  const byZone = new Map<number, { name: string; seconds: number }[]>()
  for (const zs of series) {
    for (const b of zs.buckets) {
      totals.set(b.bucket, (totals.get(b.bucket) ?? 0) + b.seconds)
      const list = byZone.get(b.bucket) ?? []
      list.push({ name: zoneNames.get(zs.zoneId) ?? `Zone ${zs.zoneId + 1}`, seconds: b.seconds })
      byZone.set(b.bucket, list)
    }
  }
  const maxSec = Math.max(1, ...totals.values())

  const W = 720
  const H = 240
  const padL = 44
  const padB = 24
  const padT = 10
  const plotW = W - padL - 8
  const plotH = H - padT - padB
  const slot = plotW / domain.length
  const barW = Math.max(6, Math.min(28, slot - 2)) // ≥2px gap between bars

  // ~4 nice y gridlines in minutes.
  const maxMin = Math.ceil(maxSec / 60)
  const step = Math.max(1, Math.ceil(maxMin / 4))
  const gridMinutes = Array.from({ length: 5 }, (_, i) => i * step).filter(
    (m) => m * 60 <= maxSec * 1.05,
  )

  const yFor = (sec: number) => padT + plotH - (sec / (step * 4 * 60)) * plotH
  const hasData = totals.size > 0

  const show = (evt: React.MouseEvent, b: number) => {
    const rect = wrapRef.current?.getBoundingClientRect()
    if (!rect) return
    setTip({
      x: evt.clientX - rect.left + 12,
      y: evt.clientY - rect.top + 12,
      title: bucketLabel(group, b),
      total: totals.get(b) ?? 0,
      perZone: (byZone.get(b) ?? []).sort((a, z) => z.seconds - a.seconds),
    })
  }

  if (!hasData) return <p className="muted">Keine Daten im gewählten Zeitraum.</p>

  return (
    <div ref={wrapRef} style={{ position: 'relative' }}>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        style={{ width: '100%', height: 'auto', display: 'block' }}
        role="img"
        aria-label="Bewässerungsdauer pro Zeitraum"
      >
        {gridMinutes.map((m) => (
          <g key={m}>
            <line
              x1={padL}
              x2={W - 8}
              y1={yFor(m * 60)}
              y2={yFor(m * 60)}
              stroke="var(--border)"
              strokeWidth={1}
            />
            <text
              x={padL - 6}
              y={yFor(m * 60) + 4}
              textAnchor="end"
              fontSize={11}
              fill="var(--text-2)"
            >
              {m}m
            </text>
          </g>
        ))}
        {domain.map((b, i) => {
          const sec = totals.get(b) ?? 0
          const x = padL + i * slot + (slot - barW) / 2
          const y = yFor(sec)
          const h = padT + plotH - y
          const r = Math.min(4, h) // rounded data-end, anchored at the baseline
          return (
            <g key={b}>
              {sec > 0 && (
                <path
                  d={`M${x},${padT + plotH} v${-(h - r)} q0,${-r} ${r},${-r} h${barW - 2 * r} q${r},0 ${r},${r} v${h - r} z`}
                  fill="var(--series-1)"
                />
              )}
              <rect
                x={padL + i * slot}
                y={padT}
                width={slot}
                height={plotH}
                fill="transparent"
                onMouseMove={(e) => show(e, b)}
                onMouseLeave={() => setTip(null)}
              />
              <text
                x={x + barW / 2}
                y={H - 8}
                textAnchor="middle"
                fontSize={11}
                fill="var(--text-2)"
              >
                {bucketTick(group, b)}
              </text>
            </g>
          )
        })}
        <line
          x1={padL}
          x2={W - 8}
          y1={padT + plotH}
          y2={padT + plotH}
          stroke="var(--text-2)"
          strokeWidth={1}
        />
      </svg>
      {tip && (
        <div className="chart-tip" style={{ left: tip.x, top: tip.y }}>
          <strong>{tip.title}</strong>
          <div>Gesamt: {fmtSeconds(tip.total)}</div>
          {tip.perZone.map((z) => (
            <div key={z.name} className="muted">
              {z.name}: {fmtSeconds(z.seconds)}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default function History() {
  const [rangeDays, setRangeDays] = useState(7)
  const [group, setGroup] = useState<Grouping>('none')
  const [entries, setEntries] = useState<LogEntry[] | null>(null)
  const [series, setSeries] = useState<ZoneSeries[] | null>(null)
  const [zones, setZones] = useState<Zone[]>([])
  const [schedNames, setSchedNames] = useState<Map<number, string>>(new Map())
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .zones()
      .then((z) => setZones(z.zones))
      .catch(() => {})
    api
      .schedules()
      .then((s) => setSchedNames(new Map(s.schedules.map((x) => [x.id, x.name]))))
      .catch(() => {})
  }, [])

  useEffect(() => {
    const end = Math.floor(Date.now() / 1000)
    const start = end - rangeDays * 86400
    if (group === 'none') {
      api
        .logEntries(start, end)
        .then((r) => {
          setEntries(r.entries)
          setSeries(null)
          setError(null)
        })
        .catch((e: Error) => setError(e.message))
    } else {
      api
        .logSeries(start, end, group)
        .then((r) => {
          setSeries(r.series)
          setEntries(null)
          setError(null)
        })
        .catch((e: Error) => setError(e.message))
    }
  }, [rangeDays, group])

  const zoneNames = useMemo(() => new Map(zones.map((z) => [z.id, z.name])), [zones])

  return (
    <>
      <h1>Verlauf</h1>
      {error && <div className="banner error">{error}</div>}

      <div className="row" style={{ marginBottom: 14 }}>
        <select
          value={rangeDays}
          onChange={(e) => setRangeDays(Number(e.target.value))}
          aria-label="Zeitraum"
        >
          {RANGES.map((r) => (
            <option key={r.days} value={r.days}>
              {r.label}
            </option>
          ))}
        </select>
        <select
          value={group}
          onChange={(e) => setGroup(e.target.value as Grouping)}
          aria-label="Ansicht"
        >
          {VIEWS.map((v) => (
            <option key={v.group} value={v.group}>
              {v.label}
            </option>
          ))}
        </select>
      </div>

      {group !== 'none' && series && (
        <div className="card">
          <h2>Bewässerungsdauer gesamt</h2>
          <BarChart series={series} group={group} zoneNames={zoneNames} />
        </div>
      )}

      {group === 'none' && entries && (
        <div className="card">
          {entries.length === 0 ? (
            <p className="muted">Keine Einträge im gewählten Zeitraum.</p>
          ) : (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Start</th>
                    <th>Zone</th>
                    <th>Auslöser</th>
                    <th className="num">Dauer</th>
                    <th className="num">Anpassung</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((e, i) => (
                    <tr key={i}>
                      <td>{fmtDate(e.start)}</td>
                      <td>{zoneNames.get(e.zoneId) ?? `Zone ${e.zoneId + 1}`}</td>
                      <td>{scheduleLabel(e.scheduleId, schedNames)}</td>
                      <td className="num">{fmtSeconds(e.seconds)}</td>
                      <td className="num">
                        {e.seasonal < 0 ? '—' : `${Math.round((e.seasonal * e.weather) / 100)} %`}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </>
  )
}

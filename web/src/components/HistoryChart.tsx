import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { SensorReading } from '../types'

type Props = {
  history: SensorReading[]
  loading?: boolean
}

type Point = {
  t: number
  label: string
  value: number
  iso: string
}

const TABLE_ROWS = 50

function toPoints(history: SensorReading[]): Point[] {
  return history.map((r) => {
    const d = new Date(r.updated_at)
    return {
      t: d.getTime(),
      label: d.toLocaleString('ko-KR', {
        month: 'numeric',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
      value: r.value,
      iso: r.updated_at,
    }
  })
}

function formatValue(v: number): string {
  return v.toLocaleString('en-US', {
    minimumFractionDigits: 3,
    maximumFractionDigits: 3,
  })
}

function summarize(data: Point[]): string {
  if (data.length === 0) return '검침 기록이 없습니다.'
  const values = data.map((d) => d.value)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const latest = data[data.length - 1]
  return `최근 7일 검침 ${data.length}건. 최저 ${formatValue(min)} m³, 최고 ${formatValue(max)} m³, 최신 ${formatValue(latest.value)} m³ (${latest.label}).`
}

export function HistoryChart({ history, loading }: Props) {
  const data = toPoints(history)
  const tableRows = data.slice(-TABLE_ROWS).reverse()

  return (
    <section className="history" aria-labelledby="history-heading">
      <div className="history__head">
        <h2 className="section-label" id="history-heading">
          검침 추이 · 7일
        </h2>
        <span className="history__count">{data.length.toLocaleString()}건</span>
      </div>

      {loading && data.length === 0 ? (
        <div className="chart-shell" aria-busy="true" aria-label="차트 불러오는 중">
          <span className="skeleton skeleton--chart" />
        </div>
      ) : data.length === 0 ? (
        <div className="chart-shell chart-shell--empty">
          <p className="empty">
            최근 7일 기록이 없습니다. 검침이 쌓이면 추이가 표시됩니다.
          </p>
        </div>
      ) : (
        <>
          <p className="sr-only">{summarize(data)}</p>

          <div
            className="chart-shell"
            role="img"
            aria-hidden="true"
          >
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="valueFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="var(--chart)" stopOpacity={0.28} />
                    <stop offset="100%" stopColor="var(--chart)" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid
                  stroke="var(--border)"
                  strokeDasharray="3 3"
                  vertical={false}
                />
                <XAxis
                  dataKey="label"
                  tick={{
                    fill: 'var(--muted)',
                    fontSize: 12,
                    fontFamily: 'var(--font-mono)',
                  }}
                  tickLine={false}
                  axisLine={{ stroke: 'var(--border)' }}
                  minTickGap={48}
                />
                <YAxis
                  domain={['auto', 'auto']}
                  tick={{
                    fill: 'var(--muted)',
                    fontSize: 12,
                    fontFamily: 'var(--font-mono)',
                  }}
                  tickLine={false}
                  axisLine={false}
                  width={56}
                  tickFormatter={(v: number) =>
                    v.toLocaleString('en-US', { maximumFractionDigits: 2 })
                  }
                />
                <Tooltip
                  contentStyle={{
                    background: 'var(--surface)',
                    border: '1px solid var(--border)',
                    borderRadius: 4,
                    fontFamily: 'var(--font-mono)',
                    fontSize: 12,
                    boxShadow: 'none',
                  }}
                  labelStyle={{ color: 'var(--muted)', marginBottom: 4 }}
                  formatter={(value) => [`${formatValue(Number(value))} m³`, '검침']}
                />
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke="var(--chart)"
                  strokeWidth={2}
                  fill="url(#valueFill)"
                  isAnimationActive={false}
                  dot={false}
                  activeDot={{ r: 3, fill: 'var(--chart)' }}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>

          <details className="history-table">
            <summary>검침 데이터 표로 보기 (최근 {tableRows.length}건)</summary>
            <div className="history-table__scroll" tabIndex={0}>
              <table>
                <caption className="sr-only">
                  최근 검침값 목록, 최신순 {tableRows.length}건
                </caption>
                <thead>
                  <tr>
                    <th scope="col">시각</th>
                    <th scope="col">검침 (m³)</th>
                  </tr>
                </thead>
                <tbody>
                  {tableRows.map((row) => (
                    <tr key={`${row.iso}-${row.value}`}>
                      <td>
                        <time dateTime={row.iso}>{row.label}</time>
                      </td>
                      <td>{formatValue(row.value)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </details>
        </>
      )}
    </section>
  )
}

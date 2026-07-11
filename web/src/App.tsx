import { lazy, Suspense, useCallback, useEffect, useState } from 'react'
import { fetchHealth, fetchHistory, fetchSensor } from './api'
import { HealthBar } from './components/HealthBar'
import { LatestReading } from './components/LatestReading'
import { SourceImage } from './components/SourceImage'
import type { HealthResponse, SensorReading, SensorResponse } from './types'

const HistoryChart = lazy(() =>
  import('./components/HistoryChart').then((m) => ({ default: m.HistoryChart })),
)

const POLL_MS = 15_000

function settledError(result: PromiseSettledResult<unknown>): string | null {
  if (result.status === 'fulfilled') return null
  const reason = result.reason
  if (reason instanceof Error && reason.message) return reason.message
  return '일부 데이터를 불러오지 못했습니다.'
}

export default function App() {
  const [sensor, setSensor] = useState<SensorResponse | null>(null)
  const [history, setHistory] = useState<SensorReading[]>([])
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lastFetchedAt, setLastFetchedAt] = useState<Date | null>(null)

  const refresh = useCallback(async (manual = false) => {
    if (manual) setRefreshing(true)
    const results = await Promise.allSettled([
      fetchSensor(),
      fetchHistory(),
      fetchHealth(),
    ])

    const [sensorResult, historyResult, healthResult] = results
    const errors: string[] = []

    if (sensorResult.status === 'fulfilled') {
      setSensor(sensorResult.value)
    } else {
      errors.push(settledError(sensorResult)!)
    }

    if (historyResult.status === 'fulfilled') {
      setHistory(historyResult.value)
    } else {
      errors.push(settledError(historyResult)!)
    }

    if (healthResult.status === 'fulfilled') {
      setHealth(healthResult.value)
    } else {
      errors.push(settledError(healthResult)!)
    }

    const unique = [...new Set(errors)]
    setError(unique.length ? unique.join(' ') : null)
    if (results.some((r) => r.status === 'fulfilled')) {
      setLastFetchedAt(new Date())
    }
    setLoading(false)
    setRefreshing(false)
  }, [])

  useEffect(() => {
    void refresh()
    const id = window.setInterval(() => {
      if (document.visibilityState === 'visible') void refresh()
    }, POLL_MS)
    return () => window.clearInterval(id)
  }, [refresh])

  return (
    <>
      <a className="skip-link" href="#main">
        본문으로 건너뛰기
      </a>
      <div className="app">
        <HealthBar
          health={health}
          lastFetchedAt={lastFetchedAt}
          loading={loading}
          refreshing={refreshing}
          onRefresh={() => void refresh(true)}
        />

        {error && (
          <p className="alert" role="alert">
            {error}
          </p>
        )}

        <main id="main">
          <section className="reading" aria-label="최신 검침">
            <LatestReading sensor={sensor} loading={loading} />
            <SourceImage
              src={sensor?.metadata?.src_image_url}
              loading={loading}
            />
          </section>

          <Suspense
            fallback={
              <div className="chart-shell" aria-busy="true">
                <span className="skeleton skeleton--chart" />
              </div>
            }
          >
            <HistoryChart history={history} loading={loading} />
          </Suspense>
        </main>

        <footer className="footer">
          <p>
            © Homin Lee &lt;
            <a href="mailto:i@homin.dev">i@homin.dev</a>
            &gt;
          </p>
        </footer>
      </div>
    </>
  )
}

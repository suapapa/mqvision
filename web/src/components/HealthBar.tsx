import type { HealthResponse } from '../types'
import { StatusChip } from './StatusChip'

type Props = {
  health: HealthResponse | null
  lastFetchedAt: Date | null
  loading: boolean
  refreshing: boolean
  onRefresh: () => void
}

function formatTime(d: Date | null): string {
  if (!d) return '—'
  return d.toLocaleString('ko-KR', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function toState(
  loading: boolean,
  ok: boolean | undefined,
): 'ok' | 'bad' | 'unknown' {
  if (loading || ok === undefined) return 'unknown'
  return ok ? 'ok' : 'bad'
}

export function HealthBar({
  health,
  lastFetchedAt,
  loading,
  refreshing,
  onRefresh,
}: Props) {
  const appOk = health ? health.status === 'ok' : undefined
  const mqttOk = health?.mqtt ? health.mqtt.connected : appOk
  const appDetail =
    health?.status !== 'ok'
      ? (health?.mqtt?.last_error ?? health?.info ?? '앱 상태가 정상이 아닙니다.')
      : health?.info
  const mqttDetail = health?.mqtt?.last_error ?? (
    health?.mqtt && !health.mqtt.connected
      ? 'MQTT 브로커에 연결되어 있지 않습니다.'
      : null
  )

  return (
    <header className="topbar">
      <div className="brand">
        <h1 className="brand__name">MQVision</h1>
        <p className="brand__tag">가스 미터 모니터링</p>
      </div>

      <div className="topbar__meta">
        <StatusChip
          label="앱"
          state={toState(loading, appOk)}
          detail={appDetail}
        />
        <StatusChip
          label="MQTT"
          state={toState(loading, mqttOk)}
          detail={mqttDetail}
        />
        <span className="clock" aria-live="polite">
          갱신 {formatTime(lastFetchedAt)}
        </span>
        <button
          type="button"
          className="refresh"
          onClick={onRefresh}
          disabled={refreshing}
          aria-busy={refreshing}
          aria-label={refreshing ? '새로고침 중' : '지금 새로고침'}
        >
          {refreshing ? '새로고침 중…' : '새로고침'}
        </button>
      </div>
    </header>
  )
}

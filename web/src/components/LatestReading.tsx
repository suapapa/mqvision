import type { SensorResponse } from '../types'

type Props = {
  sensor: SensorResponse | null
  loading: boolean
}

function formatUpdated(iso: string | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('ko-KR')
}

export function LatestReading({ sensor, loading }: Props) {
  const meta = sensor?.metadata

  return (
    <div>
      <h2 className="section-label" id="latest-heading">
        검침값
      </h2>

      {loading && !sensor ? (
        <div aria-busy="true" aria-label="검침값 불러오는 중">
          <span className="skeleton skeleton--metric" />
          <span className="skeleton skeleton--line" />
          <span className="skeleton skeleton--line skeleton--line-short" />
        </div>
      ) : !sensor ? (
        <p className="empty">
          아직 검침값이 없습니다. MQTT로 이미지가 도착하면 여기에 표시됩니다.
        </p>
      ) : (
        <>
          <p className="metric" aria-labelledby="latest-heading">
            {sensor.value.toLocaleString('en-US', {
              minimumFractionDigits: 3,
              maximumFractionDigits: 3,
            })}
            <span className="metric__unit">m³</span>
          </p>

          <dl className="meta-list">
            <div className="meta-list__row">
              <dt>갱신</dt>
              <dd>{formatUpdated(sensor.updated_at)}</dd>
            </div>
            {meta?.read && (
              <div className="meta-list__row">
                <dt>원문 읽기</dt>
                <dd>{meta.read}</dd>
              </div>
            )}
            {meta?.it_takes && (
              <div className="meta-list__row">
                <dt>AI 소요</dt>
                <dd>{meta.it_takes}</dd>
              </div>
            )}
          </dl>
        </>
      )}
    </div>
  )
}

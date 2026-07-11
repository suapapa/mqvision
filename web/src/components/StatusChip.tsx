type Props = {
  label: string
  state: 'ok' | 'bad' | 'unknown'
  detail?: string | null
}

const stateText = {
  ok: '정상',
  bad: '이상',
  unknown: '확인 중',
} as const

export function StatusChip({ label, state, detail }: Props) {
  const showDetail = state === 'bad' && Boolean(detail)

  return (
    <div className={`status status--${state}`} role="status">
      <span className="status__dot" aria-hidden />
      <span className="status__body">
        <span className="status__label">
          {label} {stateText[state]}
        </span>
        {showDetail && <span className="status__detail">{detail}</span>}
      </span>
    </div>
  )
}

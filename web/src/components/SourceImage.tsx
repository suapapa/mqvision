import { useEffect, useState } from 'react'

type Props = {
  src?: string | null
  loading?: boolean
}

export function SourceImage({ src, loading }: Props) {
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setFailed(false)
  }, [src])

  const showImage = Boolean(src) && !failed

  const handleDownload = async () => {
    if (!src) return
    try {
      const response = await fetch(src)
      const blob = await response.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `gas_meter_${new Date().toISOString().replace(/[:.]/g, '-')}.jpg`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      window.URL.revokeObjectURL(url)
    } catch (err) {
      window.open(src, '_blank')
    }
  }

  return (
    <div>
      <div className="section-header">
        <h2 className="section-label" id="image-heading">
          원본 이미지
        </h2>
        {showImage && (
          <button onClick={handleDownload} className="btn-action" aria-label="원본 이미지 다운로드">
            다운로드
          </button>
        )}
      </div>
      <div className="image-frame" aria-labelledby="image-heading">
        {loading && !src ? (
          <span className="skeleton skeleton--image" aria-hidden />
        ) : showImage ? (
          <img
            src={src!}
            alt="가스 미터 카메라 원본"
            loading="lazy"
            onError={() => setFailed(true)}
          />
        ) : (
          <p className="image-frame__empty">
            {failed
              ? '이미지를 불러오지 못했습니다. URL이 만료되었거나 접근할 수 없습니다.'
              : '저장된 카메라 이미지가 없습니다. Concierge에 업로드되면 여기에 나타납니다.'}
          </p>
        )}
      </div>
    </div>
  )
}

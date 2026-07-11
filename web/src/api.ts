import type { HealthResponse, SensorReading, SensorResponse } from './types'

export class ApiError extends Error {
  readonly status?: number

  constructor(message: string, status?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function parseJson<T>(res: Response): Promise<T> {
  return (await res.json()) as T
}

export async function fetchSensor(): Promise<SensorResponse | null> {
  let res: Response
  try {
    res = await fetch('/api/sensor')
  } catch {
    throw new ApiError('서버에 연결하지 못했습니다. 네트워크를 확인해 주세요.')
  }
  if (res.status === 425) return null
  if (!res.ok) {
    throw new ApiError(
      '최신 검침값을 불러오지 못했습니다. 잠시 후 다시 시도해 주세요.',
      res.status,
    )
  }
  return parseJson<SensorResponse>(res)
}

export async function fetchHistory(): Promise<SensorReading[]> {
  let res: Response
  try {
    res = await fetch('/api/sensors')
  } catch {
    throw new ApiError('서버에 연결하지 못했습니다. 네트워크를 확인해 주세요.')
  }
  if (!res.ok) {
    throw new ApiError(
      '검침 히스토리를 불러오지 못했습니다. 잠시 후 다시 시도해 주세요.',
      res.status,
    )
  }
  return parseJson<SensorReading[]>(res)
}

export async function fetchHealth(): Promise<HealthResponse> {
  let res: Response
  try {
    res = await fetch('/api/health')
  } catch {
    throw new ApiError('서버에 연결하지 못했습니다. 네트워크를 확인해 주세요.')
  }
  // 503 still returns a useful body
  try {
    return await parseJson<HealthResponse>(res)
  } catch {
    throw new ApiError(
      '서버 상태를 확인하지 못했습니다. 잠시 후 다시 시도해 주세요.',
      res.status,
    )
  }
}

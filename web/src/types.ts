export type SensorMetadata = {
  read?: string
  date?: string
  read_at?: string
  it_takes?: string
  src_image_url?: string
}

export type SensorResponse = {
  value: number
  updated_at: string
  metadata?: SensorMetadata
}

export type SensorReading = {
  value: number
  updated_at: string
  metadata?: SensorMetadata
}

export type HealthResponse = {
  status: string
  mqtt?: {
    connected: boolean
    last_error: string | null
  }
  sensor?: {
    last_updated: string | null
  }
  info?: string
}

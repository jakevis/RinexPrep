export type AppState = 'idle' | 'uploading' | 'processing' | 'preview' | 'ready'

export interface JobStatus {
  id: string
  status: string
  progress?: string
  error?: string
}

export interface EpochSummary {
  time_sec: number
  gps_sats: number
  total_sats: number
  avg_snr: number
}

export interface SatPosition {
  system: string
  prn: number
  azimuth: number
  elevation: number
  snr: number
}

export interface QCSummary {
  opus_ready: boolean
  score: number
  duration_hours: number
  gps_sats_mean: number
  l2_coverage_pct: number
  warnings: string[]
  failures?: string[]
}

export interface AutoTrim {
  start_sec: number
  end_sec: number
}

export interface PreviewData {
  epochs: EpochSummary[]
  skyview: SatPosition[]
  auto_trim: AutoTrim
  qc: QCSummary
  total_duration_sec: number
}

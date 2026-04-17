export type AppState = 'idle' | 'uploading' | 'processing' | 'preview' | 'ready'

export type JobEntryStatus = 'uploading' | 'parsing' | 'preview' | 'processing' | 'ready' | 'failed'

export interface JobEntry {
  id: string
  fileName: string
  status: JobEntryStatus
  error?: string
  preview?: PreviewData
  trimStart: number
  trimEnd: number
  outputFiles?: OutputFile[]
}

export interface JobStatus {
  id: string
  status: string
  progress?: string
  error?: string
}

export interface OutputFile {
  name: string
  format: string
  size: number
  label: string
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
  time_sec?: number
  freqs?: string
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
  start_time_utc?: string
  end_time_utc?: string
  sat_passes?: number
  l1_count?: number
  l2_count?: number
  l5_count?: number
  dual_freq_count?: number
  max_gap_sec?: number
  mean_snr_l1?: number
  mean_snr_l2?: number
}

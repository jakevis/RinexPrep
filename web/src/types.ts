export type AppState = 'idle' | 'uploading' | 'processing' | 'preview' | 'ready'

export interface JobStatus {
  jobId: string
  state: AppState
  progress: number
  error?: string
}

export interface EpochSummary {
  time: number
  gps: number
  glonass: number
  galileo: number
  beidou: number
  total: number
}

export interface SatPosition {
  prn: string
  constellation: 'GPS' | 'GLONASS' | 'Galileo' | 'BeiDou'
  azimuth: number
  elevation: number
}

export interface QCSummary {
  opusScore: number
  duration: number
  satelliteCount: number
  l2Coverage: number
  warnings: string[]
  failures: string[]
}

export interface AutoTrim {
  startSec: number
  endSec: number
}

export interface PreviewData {
  epochs: EpochSummary[]
  skyview: SatPosition[]
  autoTrim: AutoTrim
  qc: QCSummary
  totalDuration: number
}

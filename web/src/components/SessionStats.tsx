import { Clock, Satellite, Signal, Scissors, Timer, Radio } from 'lucide-react'
import type { PreviewData, EpochSummary } from '../types'

interface SessionStatsProps {
  preview: PreviewData
  trimStart: number
  trimEnd: number
}

function formatTime(sec: number): string {
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

function minMaxAvg(epochs: EpochSummary[], field: keyof EpochSummary): { min: number; max: number; avg: number } {
  if (epochs.length === 0) return { min: 0, max: 0, avg: 0 }
  let min = Infinity
  let max = -Infinity
  let sum = 0
  for (const e of epochs) {
    const v = e[field] as number
    if (v < min) min = v
    if (v > max) max = v
    sum += v
  }
  return { min, max, avg: sum / epochs.length }
}

function estimateInterval(epochs: EpochSummary[]): number {
  if (epochs.length < 2) return 0
  const diffs: number[] = []
  for (let i = 1; i < Math.min(epochs.length, 20); i++) {
    diffs.push(epochs[i].time_sec - epochs[i - 1].time_sec)
  }
  diffs.sort((a, b) => a - b)
  return diffs[Math.floor(diffs.length / 2)]
}

type ValueColor = 'green' | 'amber' | 'red' | 'neutral'

function colorClass(color: ValueColor): string {
  switch (color) {
    case 'green':
      return 'text-green-600 dark:text-green-400'
    case 'amber':
      return 'text-yellow-600 dark:text-yellow-400'
    case 'red':
      return 'text-red-600 dark:text-red-400'
    default:
      return 'text-gray-900 dark:text-white'
  }
}

function durationColor(sec: number): ValueColor {
  const hours = sec / 3600
  if (hours >= 4) return 'green'
  if (hours >= 2) return 'amber'
  return 'red'
}

function gpsSatsColor(avg: number): ValueColor {
  if (avg >= 6) return 'green'
  if (avg >= 4) return 'amber'
  return 'red'
}

function snrColor(avg: number): ValueColor {
  if (avg >= 35) return 'green'
  if (avg >= 25) return 'amber'
  return 'red'
}

function l2Color(pct: number): ValueColor {
  if (pct >= 80) return 'green'
  if (pct >= 50) return 'amber'
  return 'red'
}

interface StatItemProps {
  icon: React.ReactNode
  label: string
  value: string
  color?: ValueColor
}

function StatItem({ icon, label, value, color = 'neutral' }: StatItemProps) {
  return (
    <div className="flex items-start gap-2.5 py-2">
      <div className="text-gray-400 dark:text-gray-500 mt-0.5 shrink-0">{icon}</div>
      <div className="min-w-0">
        <div className="text-xs text-gray-500 dark:text-gray-400">{label}</div>
        <div className={`text-sm font-semibold ${colorClass(color)}`}>{value}</div>
      </div>
    </div>
  )
}

export default function SessionStats({ preview, trimStart, trimEnd }: SessionStatsProps) {
  const { epochs, qc, total_duration_sec: totalDur } = preview

  const gpsSats = minMaxAvg(epochs, 'gps_sats')
  const totalSats = minMaxAvg(epochs, 'total_sats')
  const snr = minMaxAvg(epochs, 'avg_snr')
  const interval = estimateInterval(epochs)

  const trimmedDuration = trimEnd - trimStart
  const trimmedFromStart = trimStart
  const trimmedFromEnd = totalDur - trimEnd

  const iconSize = 'w-4 h-4'

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
        <Radio className="w-5 h-5 text-indigo-500" />
        Session Statistics
      </h2>

      <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
        <StatItem
          icon={<Clock className={iconSize} />}
          label="Start Time"
          value={formatTime(epochs.length > 0 ? epochs[0].time_sec : 0)}
          color="neutral"
        />
        <StatItem
          icon={<Clock className={iconSize} />}
          label="End Time"
          value={formatTime(epochs.length > 0 ? epochs[epochs.length - 1].time_sec : 0)}
          color="neutral"
        />
        <StatItem
          icon={<Timer className={iconSize} />}
          label="Duration"
          value={formatTime(totalDur)}
          color={durationColor(totalDur)}
        />
        <StatItem
          icon={<Timer className={iconSize} />}
          label="Total Epochs"
          value={epochs.length.toLocaleString()}
          color="neutral"
        />
        <StatItem
          icon={<Clock className={iconSize} />}
          label="Obs Interval"
          value={interval > 0 ? `${interval.toFixed(1)}s` : '—'}
          color="neutral"
        />
        <StatItem
          icon={<Satellite className={iconSize} />}
          label="GPS Satellites"
          value={`${gpsSats.min} / ${gpsSats.avg.toFixed(1)} / ${gpsSats.max}`}
          color={gpsSatsColor(gpsSats.avg)}
        />
        <StatItem
          icon={<Satellite className={iconSize} />}
          label="Total Satellites"
          value={`${totalSats.min} / ${totalSats.avg.toFixed(1)} / ${totalSats.max}`}
          color="neutral"
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="Average SNR"
          value={snr.avg > 0 ? `${snr.avg.toFixed(1)} dB-Hz` : '—'}
          color={snrColor(snr.avg)}
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="L2 Coverage"
          value={`${qc.l2_coverage_pct.toFixed(1)}%`}
          color={l2Color(qc.l2_coverage_pct)}
        />
        <StatItem
          icon={<Scissors className={iconSize} />}
          label="Epochs after Trim"
          value={
            epochs.filter((e) => e.time_sec >= trimStart && e.time_sec <= trimEnd).length.toLocaleString()
          }
          color="neutral"
        />
        <StatItem
          icon={<Scissors className={iconSize} />}
          label="Trim Removed"
          value={`−${Math.round(trimmedFromStart)}s start, −${Math.round(trimmedFromEnd)}s end`}
          color={trimmedDuration < totalDur ? 'amber' : 'neutral'}
        />
        <StatItem
          icon={<Timer className={iconSize} />}
          label="Trimmed Duration"
          value={formatTime(trimmedDuration)}
          color={durationColor(trimmedDuration)}
        />
      </div>
    </div>
  )
}

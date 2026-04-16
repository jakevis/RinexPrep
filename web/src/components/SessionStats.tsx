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
  tooltip?: string
}

function StatItem({ icon, label, value, color = 'neutral', tooltip }: StatItemProps) {
  return (
    <div className="flex items-start gap-2.5 py-2 group relative" title={tooltip}>
      <div className="text-gray-400 dark:text-gray-500 mt-0.5 shrink-0">{icon}</div>
      <div className="min-w-0">
        <div className="text-xs text-gray-500 dark:text-gray-400 flex items-center gap-1">
          {label}
          {tooltip && (
            <span className="inline-block w-3.5 h-3.5 rounded-full bg-gray-200 dark:bg-gray-600 text-[9px] text-center leading-[14px] text-gray-500 dark:text-gray-300 cursor-help">?</span>
          )}
        </div>
        <div className={`text-sm font-semibold ${colorClass(color)}`}>{value}</div>
      </div>
    </div>
  )
}

function formatUTC(isoStr: string | undefined): string {
  if (!isoStr) return '—'
  const d = new Date(isoStr)
  return d.toISOString().replace('T', ' ').replace('Z', '').slice(0, 19) + ' UTC'
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
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6 h-full">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
        <Radio className="w-5 h-5 text-indigo-500" />
        Session Statistics
      </h2>

      <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
        <StatItem
          icon={<Clock className={iconSize} />}
          label="Start Time"
          value={formatUTC(preview.start_time_utc)}
          color="neutral"
          tooltip={`First observation epoch${preview.start_time_utc ? '' : ' (UTC not available)'}`}
        />
        <StatItem
          icon={<Clock className={iconSize} />}
          label="End Time"
          value={formatUTC(preview.end_time_utc)}
          color="neutral"
          tooltip={`Last observation epoch${preview.end_time_utc ? '' : ' (UTC not available)'}`}
        />
        <StatItem
          icon={<Timer className={iconSize} />}
          label="Duration"
          value={formatTime(totalDur)}
          color={durationColor(totalDur)}
          tooltip="Total observation session length. OPUS Static requires 2+ hours (4+ recommended). OPUS Rapid Static accepts 15 minutes minimum."
        />
        <StatItem
          icon={<Timer className={iconSize} />}
          label="Total Epochs"
          value={epochs.length.toLocaleString()}
          color="neutral"
          tooltip="Number of measurement epochs in the file. Each epoch contains observations from all visible satellites at one time instant."
        />
        <StatItem
          icon={<Clock className={iconSize} />}
          label="Obs Interval"
          value={interval > 0 ? `${interval.toFixed(1)}s` : '—'}
          color="neutral"
          tooltip="Estimated observation interval (time between epochs). OPUS expects 30-second intervals for static processing."
        />
        <StatItem
          icon={<Satellite className={iconSize} />}
          label="GPS Satellites"
          value={`${gpsSats.min} / ${gpsSats.avg.toFixed(1)} / ${gpsSats.max}`}
          color={gpsSatsColor(gpsSats.avg)}
          tooltip="Min / Average / Max GPS satellites per epoch. OPUS requires at least 4 GPS satellites. 6+ is ideal for strong solutions."
        />
        <StatItem
          icon={<Satellite className={iconSize} />}
          label="Total Satellites"
          value={`${totalSats.min} / ${totalSats.avg.toFixed(1)} / ${totalSats.max}`}
          color="neutral"
          tooltip="Min / Average / Max total satellites (all constellations) per epoch."
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="Average SNR"
          value={snr.avg > 0 ? `${snr.avg.toFixed(1)} dB-Hz` : '—'}
          color={snrColor(snr.avg)}
          tooltip="Mean carrier-to-noise ratio across all signals. Higher is better: 35+ dB-Hz is good, below 25 dB-Hz may indicate poor signal conditions."
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="L2 Coverage"
          value={`${qc.l2_coverage_pct.toFixed(1)}%`}
          color={l2Color(qc.l2_coverage_pct)}
          tooltip="Percentage of epochs with L2 (dual-frequency) GPS signals. OPUS requires dual-frequency data. 80%+ is recommended."
        />
        <StatItem
          icon={<Scissors className={iconSize} />}
          label="Epochs after Trim"
          value={
            epochs.filter((e) => e.time_sec >= trimStart && e.time_sec <= trimEnd).length.toLocaleString()
          }
          color="neutral"
          tooltip="Number of epochs remaining after applying the current trim window."
        />
        <StatItem
          icon={<Scissors className={iconSize} />}
          label="Trim Removed"
          value={`−${Math.round(trimmedFromStart)}s start, −${Math.round(trimmedFromEnd)}s end`}
          color={trimmedDuration < totalDur ? 'amber' : 'neutral'}
          tooltip="Seconds removed from the start and end of the session. Auto-trim removes unstable periods during survey setup and teardown."
        />
        <StatItem
          icon={<Timer className={iconSize} />}
          label="Trimmed Duration"
          value={formatTime(trimmedDuration)}
          color={durationColor(trimmedDuration)}
          tooltip="Effective observation duration after trimming. This is the length of data that will be written to the RINEX file."
        />
        <StatItem
          icon={<Satellite className={iconSize} />}
          label="Satellite Passes"
          value={`${preview.sat_passes ?? 0} unique`}
          color="neutral"
          tooltip="Total number of unique GPS satellites observed during the session."
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="L1 Signals"
          value={`${preview.l1_count ?? 0} sats`}
          color="neutral"
          tooltip="Number of unique satellites with L1 frequency observations."
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="L2 Signals"
          value={`${preview.l2_count ?? 0} sats`}
          color={l2Color((preview.l2_count ?? 0) / Math.max(preview.l1_count ?? 1, 1) * 100)}
          tooltip="Number of unique satellites with L2 frequency observations. Dual-frequency (L1+L2) is required by OPUS."
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="Dual Frequency"
          value={`${preview.dual_freq_count ?? 0} sats`}
          color={gpsSatsColor(preview.dual_freq_count ?? 0)}
          tooltip="Satellites with both L1 and L2 signals — essential for OPUS processing."
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="L1 Mean SNR"
          value={preview.mean_snr_l1 ? `${preview.mean_snr_l1.toFixed(1)} dB-Hz` : '—'}
          color={snrColor(preview.mean_snr_l1 ?? 0)}
          tooltip="Average signal-to-noise ratio on L1 frequency across all observations."
        />
        <StatItem
          icon={<Signal className={iconSize} />}
          label="L2 Mean SNR"
          value={preview.mean_snr_l2 ? `${preview.mean_snr_l2.toFixed(1)} dB-Hz` : '—'}
          color={snrColor(preview.mean_snr_l2 ?? 0)}
          tooltip="Average signal-to-noise ratio on L2 frequency. Lower than L1 is normal."
        />
        <StatItem
          icon={<Clock className={iconSize} />}
          label="Max Gap"
          value={preview.max_gap_sec ? `${preview.max_gap_sec.toFixed(1)}s` : '—'}
          color={preview.max_gap_sec && preview.max_gap_sec > 60 ? 'red' : preview.max_gap_sec && preview.max_gap_sec > 10 ? 'amber' : 'green'}
          tooltip="Largest time gap between consecutive observation epochs. Gaps >60s may affect OPUS processing quality."
        />
      </div>
    </div>
  )
}

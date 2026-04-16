import { ShieldCheck, AlertTriangle, XCircle } from 'lucide-react'
import type { QCSummary as QCSummaryType } from '../types'

interface QCSummaryProps {
  qc: QCSummaryType | null
}

function formatDuration(hours: number): string {
  const h = Math.floor(hours)
  const m = Math.floor((hours - h) * 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function scoreColor(score: number): string {
  if (score >= 80) return 'text-green-600 dark:text-green-400'
  if (score >= 50) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-red-600 dark:text-red-400'
}

function scoreBg(score: number): string {
  if (score >= 80) return 'bg-green-100 dark:bg-green-900/30 border-green-200 dark:border-green-800'
  if (score >= 50) return 'bg-yellow-100 dark:bg-yellow-900/30 border-yellow-200 dark:border-yellow-800'
  return 'bg-red-100 dark:bg-red-900/30 border-red-200 dark:border-red-800'
}

export default function QCSummary({ qc }: QCSummaryProps) {
  if (!qc) {
    return (
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
          <ShieldCheck className="w-5 h-5 text-indigo-500" />
          Quality Check
        </h2>
        <div className="flex flex-col items-center justify-center h-32 text-gray-400 dark:text-gray-500">
          <ShieldCheck className="w-12 h-12 mb-2 opacity-50" />
          <p>Upload a file to see quality metrics</p>
        </div>
      </div>
    )
  }

  const passed = !qc.failures || qc.failures.length === 0
  const displayScore = Math.round(qc.score)

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white flex items-center gap-2">
          <ShieldCheck className="w-5 h-5 text-indigo-500" />
          Quality Check
        </h2>
        <span
          className={`px-2.5 py-1 rounded-full text-xs font-semibold ${
            passed
              ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-400'
              : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-400'
          }`}
        >
          {passed ? 'PASS' : 'FAIL'}
        </span>
      </div>

      {/* OPUS Score */}
      <div className={`rounded-lg border p-4 mb-4 ${scoreBg(displayScore)}`}>
        <div className="text-sm text-gray-600 dark:text-gray-400 mb-1">
          OPUS Readiness Score
        </div>
        <div className={`text-3xl font-bold ${scoreColor(displayScore)}`}>
          {displayScore}
          <span className="text-base font-normal text-gray-500 dark:text-gray-400">
            /100
          </span>
        </div>
      </div>

      {/* Stats grid */}
      <div className="grid grid-cols-3 gap-3 mb-4">
        <div className="text-center">
          <div className="text-xs text-gray-500 dark:text-gray-400">Duration</div>
          <div className="text-sm font-semibold text-gray-800 dark:text-gray-200">
            {formatDuration(qc.duration_hours)}
          </div>
        </div>
        <div className="text-center">
          <div className="text-xs text-gray-500 dark:text-gray-400">Satellites</div>
          <div className="text-sm font-semibold text-gray-800 dark:text-gray-200">
            {qc.gps_sats_mean.toFixed(1)}
          </div>
        </div>
        <div className="text-center">
          <div className="text-xs text-gray-500 dark:text-gray-400">L2 Coverage</div>
          <div className="text-sm font-semibold text-gray-800 dark:text-gray-200">
            {qc.l2_coverage_pct.toFixed(1)}%
          </div>
        </div>
      </div>

      {/* Warnings */}
      {qc.warnings.length > 0 && (
        <div className="space-y-1.5 mb-3">
          {qc.warnings.map((w, i) => (
            <div
              key={i}
              className="flex items-start gap-2 text-xs text-yellow-700 dark:text-yellow-400"
            >
              <AlertTriangle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
              <span>{w}</span>
            </div>
          ))}
        </div>
      )}

      {/* Failures */}
      {qc.failures && qc.failures.length > 0 && (
        <div className="space-y-1.5">
          {qc.failures.map((f, i) => (
            <div
              key={i}
              className="flex items-start gap-2 text-xs text-red-700 dark:text-red-400"
            >
              <XCircle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
              <span>{f}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

import { useState } from 'react'
import { Download, CheckCircle2, Loader2, Send } from 'lucide-react'
import * as api from '../api'

interface DownloadPanelProps {
  jobId: string | null
  isReady: boolean
  onProcess: (format: string) => void
  onDownload: () => void
  isProcessing: boolean
}

const FORMAT_OPTIONS = [
  { value: 'rinex2', label: 'RINEX 2.11' },
  { value: 'rinex3', label: 'RINEX 3.x' },
  { value: 'both', label: 'Both' },
] as const

export default function DownloadPanel({
  jobId,
  isReady,
  onProcess,
  onDownload,
  isProcessing,
}: DownloadPanelProps) {
  const [format, setFormat] = useState<string>('rinex3')
  const [email, setEmail] = useState('')
  const [antennaType, setAntennaType] = useState('')
  const [antennaHeight, setAntennaHeight] = useState('')
  const [opusStatus, setOpusStatus] = useState<string | null>(null)
  const [opusError, setOpusError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleOpusSubmit = async (mode: string) => {
    if (!email || !antennaType) {
      setOpusError('Email and antenna type are required')
      return
    }
    if (!jobId) return
    setIsSubmitting(true)
    setOpusError(null)
    setOpusStatus(null)
    try {
      await api.submitToOPUS(jobId, {
        email,
        antenna_type: antennaType,
        height: parseFloat(antennaHeight) || 0,
        mode,
      })
      setOpusStatus(`Submitted! Results will be emailed to ${email}`)
    } catch (err) {
      setOpusError(err instanceof Error ? err.message : 'Submission failed')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6 h-full">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
        <Download className="w-5 h-5 text-indigo-500" />
        Export
      </h2>

      {/* Format selector */}
      <div className="mb-4">
        <label className="block text-sm text-gray-600 dark:text-gray-400 mb-2">
          Output Format
        </label>
        <div className="grid grid-cols-3 gap-2">
          {FORMAT_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              onClick={() => setFormat(opt.value)}
              className={`px-3 py-2 text-sm rounded-lg border transition-all ${
                format === opt.value
                  ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-950/30 text-indigo-700 dark:text-indigo-300 font-medium'
                  : 'border-gray-200 dark:border-gray-600 text-gray-600 dark:text-gray-400 hover:border-gray-300 dark:hover:border-gray-500'
              }`}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {/* Action buttons */}
      {!isReady ? (
        <button
          onClick={() => onProcess(format)}
          disabled={isProcessing}
          className="w-full flex items-center justify-center gap-2 px-4 py-3 bg-indigo-600 hover:bg-indigo-700 disabled:bg-indigo-400 text-white rounded-lg font-medium transition-colors"
        >
          {isProcessing ? (
            <>
              <Loader2 className="w-4 h-4 animate-spin" />
              Processing…
            </>
          ) : (
            <>
              <Download className="w-4 h-4" />
              Process & Convert
            </>
          )}
        </button>
      ) : (
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm text-green-600 dark:text-green-400 bg-green-50 dark:bg-green-950/20 rounded-lg px-3 py-2">
            <CheckCircle2 className="w-4 h-4" />
            Processing complete
          </div>
          <button
            onClick={onDownload}
            className="w-full flex items-center justify-center gap-2 px-4 py-3 bg-green-600 hover:bg-green-700 text-white rounded-lg font-medium transition-colors"
          >
            <Download className="w-4 h-4" />
            Download RINEX
          </button>
        </div>
      )}

      {/* OPUS Submission */}
      {isReady && (
        <div className="mt-6 pt-6 border-t border-gray-200 dark:border-gray-700">
          <h3 className="text-sm font-semibold text-gray-900 dark:text-white mb-3 flex items-center gap-2">
            <Send className="w-4 h-4 text-indigo-500" />
            Submit to OPUS
          </h3>

          <div className="space-y-3">
            <div>
              <label className="block text-xs text-gray-500 dark:text-gray-400 mb-1">
                Email (required)
              </label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="your@email.com"
                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              />
            </div>

            <div>
              <label className="block text-xs text-gray-500 dark:text-gray-400 mb-1">
                Antenna Type (required)
              </label>
              <input
                type="text"
                value={antennaType}
                onChange={(e) => setAntennaType(e.target.value)}
                placeholder="e.g., TRM57971.00     NONE"
                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              />
            </div>

            <div>
              <label className="block text-xs text-gray-500 dark:text-gray-400 mb-1">
                Antenna Height (meters)
              </label>
              <input
                type="number"
                value={antennaHeight}
                onChange={(e) => setAntennaHeight(e.target.value)}
                placeholder="0.0000"
                step="0.0001"
                min="0"
                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              />
            </div>

            <div className="grid grid-cols-2 gap-2">
              <button
                onClick={() => handleOpusSubmit('static')}
                disabled={isSubmitting}
                className="flex items-center justify-center gap-1.5 px-3 py-2.5 bg-indigo-600 hover:bg-indigo-700 disabled:bg-indigo-400 text-white text-sm rounded-lg font-medium transition-colors"
              >
                {isSubmitting ? (
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                ) : (
                  <Send className="w-3.5 h-3.5" />
                )}
                OPUS Static
              </button>
              <button
                onClick={() => handleOpusSubmit('rapid')}
                disabled={isSubmitting}
                className="flex items-center justify-center gap-1.5 px-3 py-2.5 border-2 border-indigo-600 text-indigo-600 dark:text-indigo-400 dark:border-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-950/30 disabled:opacity-50 text-sm rounded-lg font-medium transition-colors"
              >
                {isSubmitting ? (
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                ) : (
                  <Send className="w-3.5 h-3.5" />
                )}
                OPUS Rapid
              </button>
            </div>

            <p className="text-xs text-gray-400 dark:text-gray-500">
              Results will be emailed by OPUS (typically within minutes)
            </p>

            {opusStatus && (
              <div className="flex items-center gap-2 text-sm text-green-600 dark:text-green-400 bg-green-50 dark:bg-green-950/20 rounded-lg px-3 py-2">
                <CheckCircle2 className="w-4 h-4 flex-shrink-0" />
                {opusStatus}
              </div>
            )}

            {opusError && (
              <div className="text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950/20 rounded-lg px-3 py-2">
                {opusError}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

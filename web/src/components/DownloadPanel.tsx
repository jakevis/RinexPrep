import { useState } from 'react'
import { Download, CheckCircle2, Loader2 } from 'lucide-react'

interface DownloadPanelProps {
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
  isReady,
  onProcess,
  onDownload,
  isProcessing,
}: DownloadPanelProps) {
  const [format, setFormat] = useState<string>('rinex3')

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
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
    </div>
  )
}

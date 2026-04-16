import { Scissors, RotateCcw } from 'lucide-react'
import type { AutoTrim } from '../types'

interface TrimSlidersProps {
  totalDuration: number
  trimStart: number
  trimEnd: number
  autoTrim: AutoTrim | null
  onTrimChange: (start: number, end: number) => void
}

function formatTime(sec: number): string {
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

export default function TrimSliders({
  totalDuration,
  trimStart,
  trimEnd,
  autoTrim,
  onTrimChange,
}: TrimSlidersProps) {
  const isAutoTrimmed =
    autoTrim &&
    trimStart === autoTrim.start_sec &&
    trimEnd === autoTrim.end_sec

  const handleResetToAuto = () => {
    if (autoTrim) {
      onTrimChange(autoTrim.start_sec, autoTrim.end_sec)
    }
  }

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6 h-full">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white flex items-center gap-2">
          <Scissors className="w-5 h-5 text-indigo-500" />
          Trim Window
        </h2>
        {autoTrim && !isAutoTrimmed && (
          <button
            onClick={handleResetToAuto}
            className="flex items-center gap-1.5 text-xs text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 transition-colors"
          >
            <RotateCcw className="w-3.5 h-3.5" />
            Reset to auto-trim
          </button>
        )}
      </div>

      {isAutoTrimmed && (
        <div className="mb-4 px-3 py-2 bg-indigo-50 dark:bg-indigo-950/30 border border-indigo-200 dark:border-indigo-800 rounded-lg text-xs text-indigo-700 dark:text-indigo-300">
          ✨ Auto-trimmed to optimal window
        </div>
      )}

      <div className="space-y-5">
        {/* Start slider */}
        <div>
          <div className="flex justify-between text-sm text-gray-600 dark:text-gray-400 mb-2">
            <span>Start</span>
            <span className="font-mono text-xs bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded">
              {formatTime(trimStart)}
            </span>
          </div>
          <input
            type="range"
            min={0}
            max={totalDuration}
            step={1}
            value={trimStart}
            onChange={(e) => {
              const val = Number(e.target.value)
              onTrimChange(Math.min(val, trimEnd - 1), trimEnd)
            }}
            className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-lg appearance-none cursor-pointer accent-indigo-500"
          />
        </div>

        {/* End slider */}
        <div>
          <div className="flex justify-between text-sm text-gray-600 dark:text-gray-400 mb-2">
            <span>End</span>
            <span className="font-mono text-xs bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded">
              {formatTime(trimEnd)}
            </span>
          </div>
          <input
            type="range"
            min={0}
            max={totalDuration}
            step={1}
            value={trimEnd}
            onChange={(e) => {
              const val = Number(e.target.value)
              onTrimChange(trimStart, Math.max(val, trimStart + 1))
            }}
            className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-lg appearance-none cursor-pointer accent-indigo-500"
          />
        </div>

        {/* Duration summary */}
        <div className="text-center text-sm text-gray-500 dark:text-gray-400">
          Selected duration:{' '}
          <span className="font-semibold text-gray-700 dark:text-gray-300">
            {formatTime(trimEnd - trimStart)}
          </span>{' '}
          of {formatTime(totalDuration)}
        </div>
      </div>
    </div>
  )
}

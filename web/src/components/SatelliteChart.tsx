import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ReferenceArea,
  ResponsiveContainer,
} from 'recharts'
import { BarChart3 } from 'lucide-react'
import type { EpochSummary, AutoTrim } from '../types'

interface SatelliteChartProps {
  epochs: EpochSummary[] | null
  trimRange: { start: number; end: number } | null
  autoTrim: AutoTrim | null
}

function formatTime(sec: number): string {
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

export default function SatelliteChart({
  epochs,
  trimRange,
  autoTrim,
}: SatelliteChartProps) {
  if (!epochs || epochs.length === 0) {
    return (
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
          <BarChart3 className="w-5 h-5 text-indigo-500" />
          Satellite Visibility
        </h2>
        <div className="flex flex-col items-center justify-center h-48 text-gray-400 dark:text-gray-500">
          <BarChart3 className="w-12 h-12 mb-2 opacity-50" />
          <p>Upload a file to see satellite visibility</p>
        </div>
      </div>
    )
  }

  const chartData = epochs.map((e) => ({
    ...e,
    timeLabel: formatTime(e.time_sec),
  }))

  const trimStart = trimRange?.start ?? autoTrim?.start_sec
  const trimEnd = trimRange?.end ?? autoTrim?.end_sec
  const dataStart = epochs[0].time_sec
  const dataEnd = epochs[epochs.length - 1].time_sec

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
        <BarChart3 className="w-5 h-5 text-indigo-500" />
        Satellite Visibility
      </h2>
      <ResponsiveContainer width="100%" height={280}>
        <LineChart data={chartData} margin={{ top: 5, right: 20, bottom: 5, left: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#374151" opacity={0.3} />
          <XAxis
            dataKey="time_sec"
            tickFormatter={formatTime}
            stroke="#9ca3af"
            fontSize={11}
          />
          <YAxis stroke="#9ca3af" fontSize={11} />
          <Tooltip
            labelFormatter={(label) => formatTime(Number(label))}
            contentStyle={{
              backgroundColor: '#1f2937',
              border: '1px solid #374151',
              borderRadius: '8px',
              color: '#f3f4f6',
            }}
          />
          <Legend />

          {/* Trimmed-out regions (dimmed) */}
          {trimStart !== undefined && trimStart > dataStart && (
            <ReferenceArea
              x1={dataStart}
              x2={trimStart}
              fill="#ef4444"
              fillOpacity={0.08}
            />
          )}
          {trimEnd !== undefined && trimEnd < dataEnd && (
            <ReferenceArea
              x1={trimEnd}
              x2={dataEnd}
              fill="#ef4444"
              fillOpacity={0.08}
            />
          )}

          <Line type="monotone" dataKey="gps_sats" stroke="#3b82f6" name="GPS" strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="total_sats" stroke="#a855f7" name="Total" strokeWidth={2} dot={false} strokeDasharray="5 5" />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}

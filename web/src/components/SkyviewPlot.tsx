import { Compass } from 'lucide-react'
import type { SatPosition } from '../types'

interface SkyviewPlotProps {
  satellites: SatPosition[] | null
}

const CONSTELLATION_COLORS: Record<string, string> = {
  G: '#3b82f6',    // GPS
  R: '#ef4444',    // GLONASS
  E: '#22c55e',    // Galileo
  C: '#f97316',    // BeiDou
  S: '#9ca3af',    // SBAS
  J: '#a855f7',    // QZSS
}

const CONSTELLATION_NAMES: Record<string, string> = {
  G: 'GPS',
  R: 'GLONASS',
  E: 'Galileo',
  C: 'BeiDou',
  S: 'SBAS',
  J: 'QZSS',
}

const SIZE = 300
const CENTER = SIZE / 2
const RADIUS = SIZE / 2 - 30

function polarToXY(azDeg: number, elDeg: number): { x: number; y: number } {
  const r = RADIUS * (1 - elDeg / 90)
  const azRad = ((azDeg - 90) * Math.PI) / 180
  return {
    x: CENTER + r * Math.cos(azRad),
    y: CENTER + r * Math.sin(azRad),
  }
}

export default function SkyviewPlot({ satellites }: SkyviewPlotProps) {
  // Group satellites by (system, prn) to form arcs
  const arcs = new Map<string, SatPosition[]>()
  if (satellites) {
    satellites.forEach(sat => {
      const key = `${sat.system}${sat.prn}`
      if (!arcs.has(key)) arcs.set(key, [])
      arcs.get(key)!.push(sat)
    })
    // Sort each arc by time
    arcs.forEach(positions => {
      positions.sort((a, b) => (a.time_sec ?? 0) - (b.time_sec ?? 0))
    })
  }

  const hasArcs = Array.from(arcs.values()).some(positions => positions.length > 1)

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
        <Compass className="w-5 h-5 text-indigo-500" />
        Skyview Plot
      </h2>

      {!satellites || satellites.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-48 text-gray-400 dark:text-gray-500">
          <Compass className="w-12 h-12 mb-2 opacity-50" />
          <p>Upload a file to see satellite positions</p>
        </div>
      ) : (
        <div className="flex flex-col items-center">
          <svg
            viewBox={`0 0 ${SIZE} ${SIZE}`}
            className="w-full max-w-[300px]"
          >
            {/* Dark background for polar plot */}
            <circle cx={CENTER} cy={CENTER} r={RADIUS} fill="#111827" />

            {/* Concentric elevation circles */}
            {[0, 15, 30, 45, 60, 75, 90].map((el) => {
              const r = RADIUS * (1 - el / 90)
              return (
                <circle
                  key={el}
                  cx={CENTER}
                  cy={CENTER}
                  r={r}
                  fill="none"
                  stroke="#374151"
                  strokeWidth={el === 0 ? 1.5 : 0.5}
                />
              )
            })}

            {/* Cross hairs */}
            <line x1={CENTER} y1={CENTER - RADIUS} x2={CENTER} y2={CENTER + RADIUS}
              stroke="#374151" strokeWidth={0.5} />
            <line x1={CENTER - RADIUS} y1={CENTER} x2={CENTER + RADIUS} y2={CENTER}
              stroke="#374151" strokeWidth={0.5} />

            {/* Cardinal labels */}
            {(['N', 'E', 'S', 'W'] as const).map((label, i) => {
              const angle = (i * 90 - 90) * (Math.PI / 180)
              const lx = CENTER + (RADIUS + 15) * Math.cos(angle)
              const ly = CENTER + (RADIUS + 15) * Math.sin(angle)
              return (
                <text
                  key={label}
                  x={lx}
                  y={ly}
                  textAnchor="middle"
                  dominantBaseline="middle"
                  className="fill-gray-500 dark:fill-gray-400"
                  fontSize={11}
                  fontWeight={600}
                >
                  {label}
                </text>
              )
            })}

            {/* Elevation labels at 30° and 60° only */}
            {[30, 60].map((el) => {
              const r = RADIUS * (1 - el / 90)
              return (
                <text
                  key={el}
                  x={CENTER + 3}
                  y={CENTER - r + 10}
                  fill="#6b7280"
                  fontSize={8}
                >
                  {el}°
                </text>
              )
            })}

            {/* Satellite arcs or dots */}
            {hasArcs ? (
              // Arc trail mode: draw polylines per satellite
              Array.from(arcs.entries()).map(([key, positions]) => {
                const color = CONSTELLATION_COLORS[positions[0].system] ?? '#9ca3af'
                const points = positions
                  .map(p => {
                    const { x, y } = polarToXY(p.azimuth, p.elevation)
                    return `${x},${y}`
                  })
                  .join(' ')

                const last = positions[positions.length - 1]
                const { x, y } = polarToXY(last.azimuth, last.elevation)

                return (
                  <g key={key}>
                    <polyline
                      points={points}
                      fill="none"
                      stroke={color}
                      strokeWidth={1.5}
                      opacity={0.6}
                    />
                    <circle cx={x} cy={y} r={4} fill={color} opacity={0.9} />
                    <text
                      x={x} y={y - 7}
                      textAnchor="middle" fontSize={7} fontWeight={600}
                      fill={color}
                    >
                      {last.system}{last.prn}
                    </text>
                  </g>
                )
              })
            ) : (
              // Dot mode: single position per satellite (fallback)
              <>
                {satellites.map((sat, i) => {
                  const { x, y } = polarToXY(sat.azimuth, sat.elevation)
                  const color = CONSTELLATION_COLORS[sat.system] ?? '#9ca3af'
                  return (
                    <circle key={`${sat.system}${sat.prn}-${i}`} cx={x} cy={y} r={4} fill={color} opacity={0.9} />
                  )
                })}
                {Array.from(arcs.entries()).map(([key, positions]) => {
                  const sat = positions[positions.length - 1]
                  const { x, y } = polarToXY(sat.azimuth, sat.elevation)
                  const color = CONSTELLATION_COLORS[sat.system] ?? '#9ca3af'
                  return (
                    <text
                      key={`label-${key}`}
                      x={x}
                      y={y - 7}
                      textAnchor="middle"
                      fontSize={7}
                      fontWeight={600}
                      fill={color}
                    >
                      {sat.system}{sat.prn}
                    </text>
                  )
                })}
              </>
            )}
          </svg>

          {/* Legend */}
          <div className="flex flex-wrap justify-center gap-4 mt-3 text-xs">
            {Object.entries(CONSTELLATION_COLORS).map(([code, color]) => (
              <div key={code} className="flex items-center gap-1.5">
                <span
                  className="w-2.5 h-2.5 rounded-full inline-block"
                  style={{ backgroundColor: color }}
                />
                <span className="text-gray-600 dark:text-gray-400">{CONSTELLATION_NAMES[code] ?? code}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

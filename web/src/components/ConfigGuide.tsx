import { useState } from 'react'
import { Settings, Radio, Clock, MapPin, ChevronDown, ChevronRight } from 'lucide-react'

interface SectionProps {
  icon: React.ReactNode
  title: string
  children: React.ReactNode
  defaultOpen?: boolean
}

function Section({ icon, title, children, defaultOpen = false }: SectionProps) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="border-t border-gray-200 dark:border-gray-700 first:border-t-0">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 w-full py-3 text-left text-sm font-medium text-gray-700 dark:text-gray-300 hover:text-indigo-600 dark:hover:text-indigo-400 transition-colors"
      >
        {open ? (
          <ChevronDown className="w-4 h-4 text-gray-400 shrink-0" />
        ) : (
          <ChevronRight className="w-4 h-4 text-gray-400 shrink-0" />
        )}
        <span className="text-gray-400 dark:text-gray-500 shrink-0">{icon}</span>
        {title}
      </button>
      {open && <div className="pb-3 pl-8 text-sm text-gray-600 dark:text-gray-400 leading-relaxed">{children}</div>}
    </div>
  )
}

function Code({ children }: { children: React.ReactNode }) {
  return (
    <code className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-700 rounded text-xs font-mono text-indigo-600 dark:text-indigo-400">
      {children}
    </code>
  )
}

export default function ConfigGuide() {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-1 flex items-center gap-2">
        📡 Receiver Configuration Guide
      </h2>
      <p className="text-xs text-gray-500 dark:text-gray-400 mb-3">
        For best results with OPUS processing, configure your u-blox receiver to log:
      </p>

      <div className="space-y-0">
        <Section icon={<Radio className="w-4 h-4" />} title="Required Messages" defaultOpen={true}>
          <ul className="list-disc list-inside space-y-1.5">
            <li>
              <Code>RXM-RAWX</Code> — Raw GNSS measurements (pseudorange, carrier phase, doppler)
            </li>
            <li>
              <Code>RXM-SFRBX</Code> — Navigation subframe data (for <code className="text-xs">.nav</code> file generation)
            </li>
          </ul>
        </Section>

        <Section icon={<Settings className="w-4 h-4" />} title="Recommended Settings">
          <ul className="list-disc list-inside space-y-1.5">
            <li>
              <strong>Measurement rate:</strong> 1 Hz (1 measurement per second)
            </li>
            <li>
              <strong>Constellations:</strong> GPS enabled (required), GLONASS/Galileo optional
            </li>
            <li>
              <strong>Duration:</strong> OPUS Static: 2+ hours (4+ recommended). OPUS Rapid Static: 15 min minimum
            </li>
            <li>
              <strong>Antenna:</strong> Use a known IGS antenna type if possible (e.g.,{' '}
              <Code>TRM57971.00</Code>)
            </li>
            <li>
              <strong>Mount:</strong> Static survey — receiver must not move during observation
            </li>
          </ul>
        </Section>

        <Section icon={<Clock className="w-4 h-4" />} title="u-center Configuration">
          <ol className="list-decimal list-inside space-y-1.5">
            <li>
              <Code>UBX-CFG-MSG</Code>: Enable RXM-RAWX on USB/UART
            </li>
            <li>
              <Code>UBX-CFG-MSG</Code>: Enable RXM-SFRBX on USB/UART
            </li>
            <li>
              <Code>UBX-CFG-RATE</Code>: Set measurement period to 1000ms
            </li>
            <li>
              <Code>UBX-CFG-GNSS</Code>: Enable GPS (and optionally GLONASS)
            </li>
            <li>
              <Code>UBX-CFG-CFG</Code>: Save configuration to flash
            </li>
          </ol>
        </Section>

        <Section icon={<MapPin className="w-4 h-4" />} title="Output Format">
          <p>
            This tool accepts <Code>.ubx</Code> binary log files. Enable UBX protocol output (not
            NMEA) for the port you're logging from.
          </p>
        </Section>
      </div>
    </div>
  )
}

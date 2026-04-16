import { useState, useRef, useEffect } from 'react'
import { ANTENNA_TYPES } from '../antennas'
import { ChevronDown, Search, X } from 'lucide-react'

interface AntennaSelectProps {
  value: string
  onChange: (value: string) => void
}

export default function AntennaSelect({ value, onChange }: AntennaSelectProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [search, setSearch] = useState('')
  const dropdownRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  // Close on click outside
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  // Focus search input when opened
  useEffect(() => {
    if (isOpen && inputRef.current) {
      inputRef.current.focus()
    }
  }, [isOpen])

  const filtered = search
    ? ANTENNA_TYPES.filter(a => a.toLowerCase().includes(search.toLowerCase())).slice(0, 50)
    : ANTENNA_TYPES.slice(0, 50)

  return (
    <div className="relative" ref={dropdownRef}>
      {/* Trigger button */}
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-left flex items-center justify-between text-gray-900 dark:text-white"
      >
        <span className={value ? '' : 'text-gray-400'}>
          {value || 'Select antenna type...'}
        </span>
        <ChevronDown className="w-4 h-4 text-gray-400 shrink-0" />
      </button>

      {/* Dropdown */}
      {isOpen && (
        <div className="absolute z-50 mt-1 w-full bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg shadow-lg max-h-64 overflow-hidden flex flex-col">
          {/* Search input */}
          <div className="p-2 border-b border-gray-200 dark:border-gray-600 flex items-center gap-2">
            <Search className="w-4 h-4 text-gray-400 shrink-0" />
            <input
              ref={inputRef}
              type="text"
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Search antennas..."
              className="w-full text-sm bg-transparent outline-none text-gray-900 dark:text-white placeholder-gray-400"
            />
            {search && (
              <button onClick={() => setSearch('')} className="shrink-0">
                <X className="w-3.5 h-3.5 text-gray-400" />
              </button>
            )}
          </div>

          {/* Results list */}
          <div className="overflow-y-auto">
            {filtered.length === 0 ? (
              <div className="px-3 py-4 text-sm text-gray-400 text-center">
                No antennas match "{search}"
              </div>
            ) : (
              filtered.map(antenna => (
                <button
                  key={antenna}
                  type="button"
                  onClick={() => {
                    onChange(antenna)
                    setIsOpen(false)
                    setSearch('')
                  }}
                  className={`w-full text-left px-3 py-1.5 text-sm hover:bg-indigo-50 dark:hover:bg-indigo-900/30 ${
                    antenna === value
                      ? 'bg-indigo-50 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-300 font-medium'
                      : 'text-gray-700 dark:text-gray-300'
                  }`}
                >
                  <span className="font-mono text-xs">{antenna}</span>
                </button>
              ))
            )}
            {filtered.length === 50 && (
              <div className="px-3 py-2 text-xs text-gray-400 text-center border-t border-gray-200 dark:border-gray-600">
                Showing first 50 results — type to filter
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

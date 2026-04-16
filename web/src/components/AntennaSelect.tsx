import { useState, useRef, useEffect } from 'react'
import { ANTENNA_TYPES } from '../antennas'
import { ChevronDown, X } from 'lucide-react'

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
        if (!value) setSearch('')
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [value])

  const displayValue = isOpen ? search : (value || '')

  const filtered = (isOpen && search)
    ? ANTENNA_TYPES.filter(a => a.toLowerCase().includes(search.toLowerCase())).slice(0, 50)
    : ANTENNA_TYPES.slice(0, 50)

  return (
    <div className="relative" ref={dropdownRef}>
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={displayValue}
          placeholder="Type to search antennas..."
          onFocus={() => {
            setIsOpen(true)
            setSearch('')
          }}
          onChange={e => {
            setSearch(e.target.value)
            if (!isOpen) setIsOpen(true)
          }}
          onKeyDown={e => {
            if (e.key === 'Escape') {
              setIsOpen(false)
              inputRef.current?.blur()
            }
          }}
          className="w-full px-3 py-2 pr-8 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
          tabIndex={0}
        />
        {value ? (
          <button
            type="button"
            onClick={() => { onChange(''); setSearch(''); inputRef.current?.focus() }}
            className="absolute right-2 top-1/2 -translate-y-1/2"
            tabIndex={-1}
          >
            <X className="w-3.5 h-3.5 text-gray-400" />
          </button>
        ) : (
          <ChevronDown className="absolute right-2 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none" />
        )}
      </div>

      {/* Dropdown */}
      {isOpen && (
        <div className="absolute z-50 mt-1 w-full bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg shadow-lg max-h-52 overflow-y-auto">
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
              Showing first 50 — type to filter
            </div>
          )}
        </div>
      )}
    </div>
  )
}

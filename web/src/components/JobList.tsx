import { FileText, Loader2, CheckCircle2, AlertCircle, Eye, Cog } from 'lucide-react'
import type { JobEntry, JobEntryStatus } from '../types'

interface JobListProps {
  jobs: JobEntry[]
  activeJobId: string | null
  onSelectJob: (id: string) => void
}

const statusConfig: Record<JobEntryStatus, { label: string; color: string; icon: typeof Loader2 }> = {
  uploading: { label: 'Uploading', color: 'text-blue-500 bg-blue-50 dark:bg-blue-950/40', icon: Loader2 },
  parsing: { label: 'Parsing', color: 'text-amber-500 bg-amber-50 dark:bg-amber-950/40', icon: Loader2 },
  preview: { label: 'Preview', color: 'text-indigo-500 bg-indigo-50 dark:bg-indigo-950/40', icon: Eye },
  processing: { label: 'Processing', color: 'text-amber-500 bg-amber-50 dark:bg-amber-950/40', icon: Cog },
  ready: { label: 'Ready', color: 'text-green-600 bg-green-50 dark:bg-green-950/40', icon: CheckCircle2 },
  failed: { label: 'Failed', color: 'text-red-500 bg-red-50 dark:bg-red-950/40', icon: AlertCircle },
}

export default function JobList({ jobs, activeJobId, onSelectJob }: JobListProps) {
  if (jobs.length === 0) return null

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 overflow-hidden">
      <div className="px-4 py-3 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/80">
        <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
          Files ({jobs.length})
        </h3>
      </div>
      <ul className="divide-y divide-gray-100 dark:divide-gray-700/50">
        {jobs.map((job) => {
          const cfg = statusConfig[job.status]
          const Icon = cfg.icon
          const isActive = job.id === activeJobId
          const isClickable = job.status !== 'uploading'

          return (
            <li
              key={job.id}
              onClick={() => isClickable && onSelectJob(job.id)}
              className={`
                flex items-center gap-3 px-4 py-3 text-sm transition-colors
                ${isClickable ? 'cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700/40' : 'cursor-default'}
                ${isActive ? 'bg-indigo-50/70 dark:bg-indigo-950/20 border-l-3 border-indigo-500' : ''}
              `}
            >
              <FileText className="w-4 h-4 text-gray-400 dark:text-gray-500 flex-shrink-0" />
              <span className="flex-1 truncate text-gray-700 dark:text-gray-300 font-medium">
                {job.fileName}
              </span>
              <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${cfg.color}`}>
                <Icon className={`w-3 h-3 ${job.status === 'uploading' || job.status === 'parsing' || job.status === 'processing' ? 'animate-spin' : ''}`} />
                {cfg.label}
              </span>
              {job.error && (
                <span className="text-xs text-red-500 truncate max-w-[120px]" title={job.error}>
                  {job.error}
                </span>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

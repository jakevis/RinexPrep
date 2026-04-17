import { useState, useCallback, useMemo, useEffect } from 'react'
import Header from './components/Header'
import UploadZone from './components/UploadZone'
import JobList from './components/JobList'
import SatelliteChart from './components/SatelliteChart'
import SkyviewPlot from './components/SkyviewPlot'
import TrimSliders from './components/TrimSliders'
import QCSummary from './components/QCSummary'
import DownloadPanel from './components/DownloadPanel'
import SessionStats from './components/SessionStats'
import ConfigGuide from './components/ConfigGuide'
import * as api from './api'
import type { JobEntry } from './types'
import { Loader2, X } from 'lucide-react'

function App() {
  const [jobs, setJobs] = useState<JobEntry[]>([])
  const [activeJobId, setActiveJobId] = useState<string | null>(null)
  const [uploadProgress, setUploadProgress] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [isProcessing, setIsProcessing] = useState(false)
  const [progressMessage, setProgressMessage] = useState<string | null>(null)
  const [expandedPanel, setExpandedPanel] = useState<string | null>(null)
  const [version, setVersion] = useState<string>('')

  useEffect(() => {
    fetch('/api/v1/version')
      .then(r => r.json())
      .then(d => setVersion(d.version))
      .catch(() => {})
  }, [])

  const activeJob = useMemo(
    () => jobs.find((j) => j.id === activeJobId) ?? null,
    [jobs, activeJobId],
  )

  // Derive the app-level display state from the active job
  const appState = useMemo(() => {
    if (!activeJob) {
      // Show uploading spinner if any job is still uploading
      if (jobs.some((j) => j.status === 'uploading')) return 'uploading' as const
      return 'idle' as const
    }
    switch (activeJob.status) {
      case 'uploading': return 'uploading' as const
      case 'parsing': return 'processing' as const
      case 'processing': return 'processing' as const
      case 'preview': return 'preview' as const
      case 'ready': return 'ready' as const
      case 'failed': return 'idle' as const
      default: return 'idle' as const
    }
  }, [activeJob, jobs])

  const updateJob = useCallback((id: string, patch: Partial<JobEntry>) => {
    setJobs((prev) => prev.map((j) => (j.id === id ? { ...j, ...patch } : j)))
  }, [])

  const uploadAndPollFile = useCallback(async (file: File) => {
    // Create a temporary ID for the uploading state
    const tempId = `pending-${Date.now()}-${Math.random().toString(36).slice(2)}`
    const entry: JobEntry = {
      id: tempId,
      fileName: file.name,
      status: 'uploading',
      trimStart: 0,
      trimEnd: 0,
    }
    setJobs((prev) => [...prev, entry])
    setActiveJobId(tempId)
    setUploadProgress(0)
    setError(null)

    try {
      const { jobId: newJobId } = await api.uploadFile(file, setUploadProgress)
      // Replace temp entry with real job ID
      setJobs((prev) =>
        prev.map((j) => (j.id === tempId ? { ...j, id: newJobId, status: 'parsing' } : j)),
      )
      setActiveJobId(newJobId)
      setUploadProgress(null)

      // Poll for parse completion
      await new Promise<void>((resolve, reject) => {
        const pollInterval = setInterval(async () => {
          try {
            const status = await api.getJobStatus(newJobId)
            if (status.progress) {
              setProgressMessage(status.progress)
            }
            if (status.status === 'preview') {
              clearInterval(pollInterval)
              const previewData = await api.getPreview(newJobId)
              setJobs((prev) =>
                prev.map((j) =>
                  j.id === newJobId
                    ? {
                        ...j,
                        status: 'preview',
                        preview: previewData,
                        trimStart: previewData.auto_trim?.start_sec ?? 0,
                        trimEnd: previewData.auto_trim?.end_sec ?? previewData.total_duration_sec ?? 0,
                      }
                    : j,
                ),
              )
              setProgressMessage(null)
              resolve()
            } else if (status.status === 'failed') {
              clearInterval(pollInterval)
              setJobs((prev) =>
                prev.map((j) =>
                  j.id === newJobId ? { ...j, status: 'failed', error: status.error ?? 'Processing failed' } : j,
                ),
              )
              setProgressMessage(null)
              resolve() // resolve (not reject) so the queue continues
            }
          } catch {
            clearInterval(pollInterval)
            setJobs((prev) =>
              prev.map((j) =>
                j.id === newJobId ? { ...j, status: 'failed', error: 'Lost connection to server' } : j,
              ),
            )
            setProgressMessage(null)
            reject(new Error('Lost connection'))
          }
        }, 1000)
      })
    } catch (err) {
      // Upload itself failed — remove the temp entry
      setJobs((prev) => prev.filter((j) => j.id !== tempId))
      setUploadProgress(null)
      setError(err instanceof Error ? err.message : 'Upload failed')
    }
  }, [])

  const handleFilesSelected = useCallback(
    async (files: File[]) => {
      // Process files sequentially
      for (const file of files) {
        await uploadAndPollFile(file)
      }
    },
    [uploadAndPollFile],
  )

  const handleSelectJob = useCallback((id: string) => {
    setActiveJobId(id)
    setError(null)
  }, [])

  const handleTrimChange = useCallback(
    (start: number, end: number) => {
      if (!activeJobId) return
      updateJob(activeJobId, {
        trimStart: start,
        trimEnd: end,
        // Reset to preview if was ready (user changed trim)
        ...(activeJob?.status === 'ready' ? { status: 'preview', outputFiles: undefined } : {}),
      })
    },
    [activeJobId, activeJob, updateJob],
  )

  const handleProcess = useCallback(async () => {
    if (!activeJobId || !activeJob) return
    setIsProcessing(true)
    setError(null)
    updateJob(activeJobId, { status: 'processing' })
    try {
      await api.submitTrim(activeJobId, activeJob.trimStart, activeJob.trimEnd)
      await api.processJob(activeJobId)
      const filesResp = await api.getOutputFiles(activeJobId)
      updateJob(activeJobId, { status: 'ready', outputFiles: filesResp.files })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Processing failed')
      updateJob(activeJobId, { status: 'preview' })
    } finally {
      setIsProcessing(false)
    }
  }, [activeJobId, activeJob, updateJob])

  const handleDownload = useCallback(
    async (format?: string) => {
      if (!activeJobId) return
      try {
        const blob = await api.downloadResult(activeJobId, format)
        const baseName = activeJob?.fileName?.replace(/\.ubx$/i, '') ?? 'rinex_output'
        const ext = format === 'rinex2' ? '.obs' : format === 'rinex3' ? '.rnx' : '.zip'
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `${baseName}${ext}`
        a.click()
        URL.revokeObjectURL(url)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Download failed')
      }
    },
    [activeJobId, activeJob],
  )

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 transition-colors">
      <Header />

      <main className="max-w-6xl mx-auto px-4 py-8 space-y-6">
        {/* Error banner */}
        {error && (
          <div className="bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800 rounded-xl px-4 py-3 text-sm text-red-700 dark:text-red-400">
            {error}
          </div>
        )}

        {/* Upload section */}
        <UploadZone
          onFilesSelected={handleFilesSelected}
          uploadProgress={uploadProgress}
          isUploading={jobs.some((j) => j.status === 'uploading')}
        />

        {/* Job list */}
        <JobList
          jobs={jobs}
          activeJobId={activeJobId}
          onSelectJob={handleSelectJob}
        />

        {/* Processing spinner */}
        {appState === 'processing' && (
          <div className="flex flex-col items-center py-12 text-gray-500 dark:text-gray-400">
            <Loader2 className="w-10 h-10 animate-spin text-indigo-500 mb-3" />
            <p className="font-medium">Processing your file…</p>
            <p className="text-sm">{progressMessage ?? 'Parsing UBX data and analyzing satellites'}</p>
          </div>
        )}

        {/* Preview & controls */}
        {(appState === 'preview' || appState === 'ready') && activeJob?.preview && (
          <>
            {/* Charts row */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-stretch">
              <div className="lg:col-span-2 flex">
                <div className="w-full">
                  <SatelliteChart
                    epochs={activeJob.preview.epochs}
                    trimRange={{ start: activeJob.trimStart, end: activeJob.trimEnd }}
                    autoTrim={activeJob.preview.auto_trim}
                    onExpand={() => setExpandedPanel('chart')}
                  />
                </div>
              </div>
              <div className="flex">
                <div className="w-full">
                  <SkyviewPlot
                    satellites={activeJob.preview.skyview}
                    onExpand={() => setExpandedPanel('skyview')}
                  />
                </div>
              </div>
            </div>

            {/* Controls row */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-stretch">
              <div className="flex">
                <div className="w-full">
                  <SessionStats
                    preview={activeJob.preview}
                    trimStart={activeJob.trimStart}
                    trimEnd={activeJob.trimEnd}
                  />
                </div>
              </div>
              <div className="flex flex-col gap-4">
                <QCSummary qc={activeJob.preview.qc} />
                <TrimSliders
                  totalDuration={activeJob.preview.total_duration_sec}
                  trimStart={activeJob.trimStart}
                  trimEnd={activeJob.trimEnd}
                  autoTrim={activeJob.preview.auto_trim}
                  startTimeUTC={activeJob.preview.start_time_utc}
                  onTrimChange={handleTrimChange}
                />
              </div>
              <div className="flex">
                <div className="w-full">
                  <DownloadPanel
                    jobId={activeJobId}
                    isReady={appState === 'ready'}
                    onProcess={handleProcess}
                    onDownload={handleDownload}
                    isProcessing={isProcessing}
                    outputFiles={activeJob.outputFiles}
                  />
                </div>
              </div>
            </div>

            {/* Expanded panel overlay */}
            {expandedPanel && (
              <div
                className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-6"
                onClick={() => setExpandedPanel(null)}
              >
                <div
                  className="bg-white dark:bg-gray-800 rounded-2xl shadow-2xl w-full max-w-6xl max-h-[90vh] overflow-auto p-8 relative"
                  onClick={e => e.stopPropagation()}
                >
                  <button
                    className="absolute top-4 right-4 p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700"
                    onClick={() => setExpandedPanel(null)}
                  >
                    <X className="w-5 h-5 text-gray-500" />
                  </button>
                  {expandedPanel === 'chart' && (
                    <div className="h-[70vh]">
                      <SatelliteChart
                        epochs={activeJob.preview.epochs}
                        trimRange={{ start: activeJob.trimStart, end: activeJob.trimEnd }}
                        autoTrim={activeJob.preview.auto_trim}
                      />
                    </div>
                  )}
                  {expandedPanel === 'skyview' && (
                    <SkyviewPlot satellites={activeJob.preview.skyview} />
                  )}
                </div>
              </div>
            )}
          </>
        )}

        {/* Idle state - show config guide and placeholders */}
        {appState === 'idle' && jobs.length === 0 && (
          <>
            <ConfigGuide />
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-2">
                <SatelliteChart epochs={null} trimRange={null} autoTrim={null} />
              </div>
              <div>
                <SkyviewPlot satellites={null} />
              </div>
            </div>
          </>
        )}
      </main>

      <footer className="text-xs text-gray-400 dark:text-gray-600 py-6 border-t border-gray-200 dark:border-gray-800 px-4">
        <div className="max-w-6xl mx-auto flex justify-between items-center">
          <span>RinexPrep — Open-source GNSS processing tool</span>
          {version && <a href={`https://github.com/jakevis/RinexPrep/releases/tag/${version}`} target="_blank" rel="noopener noreferrer" className="hover:text-gray-600 dark:hover:text-gray-400 transition-colors">{version}</a>}
        </div>
      </footer>
    </div>
  )
}

export default App

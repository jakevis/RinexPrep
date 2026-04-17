import { useState, useCallback } from 'react'
import Header from './components/Header'
import UploadZone from './components/UploadZone'
import SatelliteChart from './components/SatelliteChart'
import SkyviewPlot from './components/SkyviewPlot'
import TrimSliders from './components/TrimSliders'
import QCSummary from './components/QCSummary'
import DownloadPanel from './components/DownloadPanel'
import SessionStats from './components/SessionStats'
import ConfigGuide from './components/ConfigGuide'
import * as api from './api'
import type { AppState, PreviewData, OutputFile } from './types'
import { Loader2, X } from 'lucide-react'

function App() {
  const [appState, setAppState] = useState<AppState>('idle')
  const [jobId, setJobId] = useState<string | null>(null)
  const [uploadProgress, setUploadProgress] = useState<number | null>(null)
  const [preview, setPreview] = useState<PreviewData | null>(null)
  const [trimStart, setTrimStart] = useState(0)
  const [trimEnd, setTrimEnd] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [isProcessing, setIsProcessing] = useState(false)
  const [progressMessage, setProgressMessage] = useState<string | null>(null)
  const [expandedPanel, setExpandedPanel] = useState<string | null>(null)
  const [outputFiles, setOutputFiles] = useState<OutputFile[] | undefined>()

  const handleFileSelected = useCallback(async (file: File) => {
    setError(null)
    setAppState('uploading')
    setUploadProgress(0)

    try {
      const { jobId: newJobId } = await api.uploadFile(file, setUploadProgress)
      setJobId(newJobId)
      setUploadProgress(null)
      setAppState('processing')

      // Poll for job completion
      const pollInterval = setInterval(async () => {
        try {
          const status = await api.getJobStatus(newJobId)
          if (status.progress) {
            setProgressMessage(status.progress)
          }
          if (status.status === 'preview') {
            clearInterval(pollInterval)
            const previewData = await api.getPreview(newJobId)
            setPreview(previewData)
            setTrimStart(previewData.auto_trim?.start_sec ?? 0)
            setTrimEnd(previewData.auto_trim?.end_sec ?? previewData.total_duration_sec ?? 0)
            setProgressMessage(null)
            setAppState('preview')
          } else if (status.status === 'failed') {
            clearInterval(pollInterval)
            setError(status.error ?? 'Processing failed')
            setProgressMessage(null)
            setAppState('idle')
          }
        } catch {
          clearInterval(pollInterval)
          setError('Lost connection to server')
          setProgressMessage(null)
          setAppState('idle')
        }
      }, 1000)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed')
      setUploadProgress(null)
      setAppState('idle')
    }
  }, [])

  const handleTrimChange = useCallback((start: number, end: number) => {
    setTrimStart(start)
    setTrimEnd(end)
  }, [])

  const handleProcess = useCallback(
    async () => {
      if (!jobId) return
      setIsProcessing(true)
      setError(null)
      try {
        await api.submitTrim(jobId, trimStart, trimEnd)
        await api.processJob(jobId)
        const filesResp = await api.getOutputFiles(jobId)
        setOutputFiles(filesResp.files)
        setAppState('ready')
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Processing failed')
      } finally {
        setIsProcessing(false)
      }
    },
    [jobId, trimStart, trimEnd],
  )

  const handleDownload = useCallback(async (format?: string) => {
    if (!jobId) return
    try {
      const blob = await api.downloadResult(jobId, format)
      const ext = format === 'rinex2' ? '.obs' : format === 'rinex3' ? '.rnx' : '.zip'
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `rinex_output${ext}`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Download failed')
    }
  }, [jobId])

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
          onFileSelected={handleFileSelected}
          uploadProgress={uploadProgress}
          isUploading={appState === 'uploading'}
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
        {(appState === 'preview' || appState === 'ready') && preview && (
          <>
            {/* Charts row */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-stretch">
              <div className="lg:col-span-2 flex">
                <div className="w-full">
                  <SatelliteChart
                    epochs={preview.epochs}
                    trimRange={{ start: trimStart, end: trimEnd }}
                    autoTrim={preview.auto_trim}
                    onExpand={() => setExpandedPanel('chart')}
                  />
                </div>
              </div>
              <div className="flex">
                <div className="w-full">
                  <SkyviewPlot
                    satellites={preview.skyview}
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
                    preview={preview}
                    trimStart={trimStart}
                    trimEnd={trimEnd}
                  />
                </div>
              </div>
              <div className="flex flex-col gap-4">
                <QCSummary qc={preview.qc} />
                <TrimSliders
                  totalDuration={preview.total_duration_sec}
                  trimStart={trimStart}
                  trimEnd={trimEnd}
                  autoTrim={preview.auto_trim}
                  startTimeUTC={preview.start_time_utc}
                  onTrimChange={handleTrimChange}
                />
              </div>
              <div className="flex">
                <div className="w-full">
                  <DownloadPanel
                    jobId={jobId}
                    isReady={appState === 'ready'}
                    onProcess={handleProcess}
                    onDownload={handleDownload}
                    isProcessing={isProcessing}
                    outputFiles={outputFiles}
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
                        epochs={preview.epochs}
                        trimRange={{ start: trimStart, end: trimEnd }}
                        autoTrim={preview.auto_trim}
                      />
                    </div>
                  )}
                  {expandedPanel === 'skyview' && (
                    <SkyviewPlot satellites={preview.skyview} />
                  )}
                </div>
              </div>
            )}
          </>
        )}

        {/* Idle state - show config guide and placeholders */}
        {appState === 'idle' && (
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

      <footer className="text-center text-xs text-gray-400 dark:text-gray-600 py-6 border-t border-gray-200 dark:border-gray-800">
        RinexPrep — Open-source GNSS processing tool
      </footer>
    </div>
  )
}

export default App

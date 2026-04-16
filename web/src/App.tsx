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
import type { AppState, PreviewData } from './types'
import { Loader2 } from 'lucide-react'

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
            setTrimStart(previewData.auto_trim.start_sec)
            setTrimEnd(previewData.auto_trim.end_sec)
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
    async (format: string) => {
      if (!jobId) return
      setIsProcessing(true)
      setError(null)
      try {
        await api.submitTrim(jobId, trimStart, trimEnd)
        await api.processJob(jobId, format)
        setAppState('ready')
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Processing failed')
      } finally {
        setIsProcessing(false)
      }
    },
    [jobId, trimStart, trimEnd],
  )

  const handleDownload = useCallback(async () => {
    if (!jobId) return
    try {
      const blob = await api.downloadResult(jobId)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `rinex_${jobId}.zip`
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
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-2">
                <SatelliteChart
                  epochs={preview.epochs}
                  trimRange={{ start: trimStart, end: trimEnd }}
                  autoTrim={preview.auto_trim}
                />
              </div>
              <div>
                <SkyviewPlot satellites={preview.skyview} />
              </div>
            </div>

            {/* Controls row */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div>
                <SessionStats
                  preview={preview}
                  trimStart={trimStart}
                  trimEnd={trimEnd}
                />
              </div>
              <div>
                <TrimSliders
                  totalDuration={preview.total_duration_sec}
                  trimStart={trimStart}
                  trimEnd={trimEnd}
                  autoTrim={preview.auto_trim}
                  onTrimChange={handleTrimChange}
                />
              </div>
              <div>
                <QCSummary qc={preview.qc} />
              </div>
            </div>

            {/* Download row */}
            <div className="flex justify-center">
              <div className="w-full max-w-md">
                <DownloadPanel
                  isReady={appState === 'ready'}
                  onProcess={handleProcess}
                  onDownload={handleDownload}
                  isProcessing={isProcessing}
                />
              </div>
            </div>
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

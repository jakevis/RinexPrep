import type { JobStatus, PreviewData, OutputFile } from './types'

const API_BASE = '/api/v1'

export async function uploadFile(
  file: File,
  onProgress?: (pct: number) => void,
): Promise<{ jobId: string }> {
  const formData = new FormData()
  formData.append('file', file)

  const xhr = new XMLHttpRequest()
  const result = await new Promise<{ jobId: string }>((resolve, reject) => {
    xhr.open('POST', `${API_BASE}/upload`)
    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable && onProgress) {
        onProgress(Math.round((e.loaded / e.total) * 100))
      }
    })
    xhr.addEventListener('load', () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(JSON.parse(xhr.responseText))
      } else {
        reject(new Error(`Upload failed: ${xhr.statusText}`))
      }
    })
    xhr.addEventListener('error', () => reject(new Error('Upload failed')))
    xhr.send(formData)
  })

  return result
}

export async function getJobStatus(jobId: string): Promise<JobStatus> {
  const res = await fetch(`${API_BASE}/jobs/${jobId}/status`)
  if (!res.ok) throw new Error(`Failed to get job status: ${res.statusText}`)
  return res.json()
}

export async function getPreview(jobId: string): Promise<PreviewData> {
  const res = await fetch(`${API_BASE}/jobs/${jobId}/preview`)
  if (!res.ok) throw new Error(`Failed to get preview: ${res.statusText}`)
  return res.json()
}

export async function submitTrim(
  jobId: string,
  start: number,
  end: number,
): Promise<void> {
  const res = await fetch(`${API_BASE}/jobs/${jobId}/trim`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ start_sec: start, end_sec: end }),
  })
  if (!res.ok) throw new Error(`Failed to submit trim: ${res.statusText}`)
}

export async function processJob(jobId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/jobs/${jobId}/process`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({}),
  })
  if (!res.ok) throw new Error(`Failed to process job: ${res.statusText}`)
}

export async function getOutputFiles(jobId: string): Promise<{ files: OutputFile[] }> {
  const res = await fetch(`${API_BASE}/jobs/${jobId}/files`)
  if (!res.ok) throw new Error('Failed to get files')
  return res.json()
}

export async function downloadResult(jobId: string, format?: string): Promise<Blob> {
  const params = format ? `?format=${format}` : ''
  const res = await fetch(`${API_BASE}/jobs/${jobId}/download${params}`)
  if (!res.ok) throw new Error(`Failed to download: ${res.statusText}`)
  return res.blob()
}

export async function submitToOPUS(
  jobId: string,
  data: { email: string; antenna_type: string; height: number; mode: string },
): Promise<{ status: string; message: string; queue_position?: string; processor?: string; rinex_file?: string }> {
  const res = await fetch(`${API_BASE}/jobs/${jobId}/opus`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const err = await res.json()
    throw new Error(err.message || 'OPUS submission failed')
  }
  return res.json()
}

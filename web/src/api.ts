import type { JobStatus, PreviewData, OutputFile } from './types'

const API_BASE = '/api/v1'
const CHUNK_SIZE = 50 * 1024 * 1024 // 50 MB — well under Cloudflare's 100 MB limit

export async function uploadFile(
  file: File,
  onProgress?: (pct: number) => void,
): Promise<{ jobId: string }> {
  if (file.size <= CHUNK_SIZE) {
    return uploadSingle(file, onProgress)
  }
  return uploadChunked(file, onProgress)
}

// uploadSingle handles small files with a single POST (existing behavior).
async function uploadSingle(
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

// uploadChunked splits large files into chunks and uploads sequentially.
async function uploadChunked(
  file: File,
  onProgress?: (pct: number) => void,
): Promise<{ jobId: string }> {
  // 1. Initialize the upload.
  const initRes = await fetch(`${API_BASE}/upload/init`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filename: file.name, size: file.size }),
  })
  if (!initRes.ok) {
    const err = await initRes.json().catch(() => ({ error: initRes.statusText }))
    throw new Error(err.error || 'Upload init failed')
  }
  const { jobId } = await initRes.json()

  // 2. Upload chunks sequentially.
  const totalChunks = Math.ceil(file.size / CHUNK_SIZE)
  let offset = 0

  for (let i = 0; i < totalChunks; i++) {
    const end = Math.min(offset + CHUNK_SIZE, file.size)
    const blob = file.slice(offset, end)

    const chunkForm = new FormData()
    chunkForm.append('chunk', blob)

    await new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('POST', `${API_BASE}/upload/${jobId}/chunk`)
      xhr.setRequestHeader('X-Upload-Offset', String(offset))

      xhr.upload.addEventListener('progress', (e) => {
        if (e.lengthComputable && onProgress) {
          const chunkProgress = e.loaded / e.total
          const overallProgress = (i + chunkProgress) / totalChunks * 100
          onProgress(Math.round(overallProgress))
        }
      })
      xhr.addEventListener('load', () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          resolve()
        } else {
          reject(new Error(`Chunk ${i + 1}/${totalChunks} failed: ${xhr.responseText}`))
        }
      })
      xhr.addEventListener('error', () =>
        reject(new Error(`Chunk ${i + 1}/${totalChunks} upload failed`)),
      )
      xhr.send(chunkForm)
    })

    offset = end
  }

  // 3. Complete the upload.
  const completeRes = await fetch(`${API_BASE}/upload/${jobId}/complete`, {
    method: 'POST',
  })
  if (!completeRes.ok) {
    const err = await completeRes.json().catch(() => ({ error: completeRes.statusText }))
    throw new Error(err.error || 'Upload completion failed')
  }

  if (onProgress) onProgress(100)
  return { jobId }
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
): Promise<{ status: string; message: string; queue_position?: string; processor?: string; rinex_file?: string; details?: Record<string, string> }> {
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

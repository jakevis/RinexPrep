import { useCallback } from 'react'
import { useDropzone } from 'react-dropzone'
import { Upload, FileUp } from 'lucide-react'

interface UploadZoneProps {
  onFilesSelected: (files: File[]) => void
  uploadProgress: number | null
  isUploading: boolean
}

export default function UploadZone({
  onFilesSelected,
  uploadProgress,
  isUploading,
}: UploadZoneProps) {
  const onDrop = useCallback(
    (acceptedFiles: File[]) => {
      if (acceptedFiles.length > 0) {
        onFilesSelected(acceptedFiles)
      }
    },
    [onFilesSelected],
  )

  const { getRootProps, getInputProps, isDragActive } =
    useDropzone({
      onDrop,
      accept: { 'application/octet-stream': ['.ubx'] },
      multiple: true,
      disabled: isUploading,
    })


  return (
    <div
      {...getRootProps()}
      className={`
        relative border-2 border-dashed rounded-xl p-8 text-center cursor-pointer
        transition-all duration-200 ease-in-out
        ${isDragActive
          ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-950/30'
          : 'border-gray-300 dark:border-gray-600 hover:border-indigo-400 dark:hover:border-indigo-500 bg-gray-50 dark:bg-gray-800/50'
        }
        ${isUploading ? 'pointer-events-none opacity-70' : ''}
      `}
    >
      <input {...getInputProps()} />

      <div className="flex flex-col items-center gap-3">
        {isUploading ? (
          <FileUp className="w-10 h-10 text-indigo-500 animate-bounce" />
        ) : (
          <Upload className="w-10 h-10 text-gray-400 dark:text-gray-500" />
        )}

        {isDragActive ? (
          <p className="text-indigo-600 dark:text-indigo-400 font-medium">
            Drop the file here…
          </p>
        ) : (
          <div>
            <p className="text-gray-700 dark:text-gray-300 font-medium">
              Drag & drop <span className="font-mono text-sm">.ubx</span> files here
            </p>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
              or click to browse
            </p>
          </div>
        )}

        
      </div>

      {uploadProgress !== null && (
        <div className="mt-4">
          <div className="flex justify-between text-xs text-gray-500 dark:text-gray-400 mb-1">
            <span>Uploading…</span>
            <span>{uploadProgress}%</span>
          </div>
          <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2 overflow-hidden">
            <div
              className="bg-indigo-500 h-2 rounded-full transition-all duration-300 ease-out"
              style={{ width: `${uploadProgress}%` }}
            />
          </div>
        </div>
      )}
    </div>
  )
}

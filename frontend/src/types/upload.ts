// ─── API Response Types ──────────────────────────────────────────────────────

export interface InitUploadResponse {
  upload_id: string;
  chunk_size: number;
  total_chunks: number;
}

export interface ChunkUploadResponse {
  upload_id: string;
  chunk_index: number;
  uploaded_chunks: number;
  total_chunks: number;
}

export interface UploadStatusResponse {
  upload_id: string;
  status: 'initialized' | 'in_progress' | 'completed' | 'failed';
  file_name: string;
  file_size: number;
  total_chunks: number;
  uploaded_chunks: number[];
  next_chunk_index: number;
  progress_percent: number;
}

export interface CompleteUploadResponse {
  upload_id: string;
  file_name: string;
  location: string;
  message: string;
}

export interface ApiErrorResponse {
  error: string;
  details?: string;
}

// ─── Client-Side Upload State ────────────────────────────────────────────────

export type UploadPhase =
  | 'idle'
  | 'hashing'
  | 'initializing'
  | 'uploading'
  | 'paused'
  | 'completing'
  | 'done'
  | 'error';

export interface UploadState {
  phase: UploadPhase;
  file: File | null;
  uploadId: string | null;
  totalChunks: number;
  chunkSize: number;
  uploadedChunks: Set<number>;
  currentChunkIndex: number;
  bytesSent: number;
  speedBps: number;       // bytes per second (rolling avg)
  errorMessage: string | null;
  location: string | null; // final object key on success
  sha256: string | null;
}

import type {
  ChunkUploadResponse,
  CompleteUploadResponse,
  InitUploadResponse,
  UploadStatusResponse,
} from '../types/upload';

const BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body?.error ?? `HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}

/** Stage 1 – Initialise a new upload session. */
export async function initUpload(params: {
  fileName: string;
  fileSize: number;
  mimeType: string;
  sha256Hash?: string;
}): Promise<InitUploadResponse> {
  const res = await fetch(`${BASE}/upload/init`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      file_name: params.fileName,
      file_size: params.fileSize,
      mime_type: params.mimeType,
      sha256_hash: params.sha256Hash ?? '',
    }),
  });
  return handleResponse<InitUploadResponse>(res);
}

/** Stage 2 – Upload a single chunk (raw bytes). */
export async function uploadChunk(
  uploadId: string,
  chunkIndex: number,
  chunk: Blob,
): Promise<ChunkUploadResponse> {
  const res = await fetch(
    `${BASE}/upload/chunk?upload_id=${encodeURIComponent(uploadId)}&chunk_index=${chunkIndex}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: chunk,
    },
  );
  return handleResponse<ChunkUploadResponse>(res);
}

/** Stage 3 – Finalise and merge all chunks. */
export async function completeUpload(uploadId: string): Promise<CompleteUploadResponse> {
  const res = await fetch(
    `${BASE}/upload/complete?upload_id=${encodeURIComponent(uploadId)}`,
    { method: 'POST' },
  );
  return handleResponse<CompleteUploadResponse>(res);
}

/** Resume helper – fetch current server-side progress. */
export async function getUploadStatus(uploadId: string): Promise<UploadStatusResponse> {
  const res = await fetch(
    `${BASE}/upload/status?upload_id=${encodeURIComponent(uploadId)}`,
  );
  return handleResponse<UploadStatusResponse>(res);
}

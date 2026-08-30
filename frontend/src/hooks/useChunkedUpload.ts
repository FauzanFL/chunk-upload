import { useCallback, useRef, useState } from 'react';
import { completeUpload, getUploadStatus, initUpload, uploadChunk } from '../services/uploadApi';
import type { UploadState } from '../types/upload';
import { sha256File } from '../utils/hash';

const ROLLING_WINDOW = 5; // samples for speed average

function makeInitialState(): UploadState {
  return {
    phase: 'idle',
    file: null,
    uploadId: null,
    totalChunks: 0,
    chunkSize: 0,
    uploadedChunks: new Set(),
    currentChunkIndex: 0,
    bytesSent: 0,
    speedBps: 0,
    errorMessage: null,
    location: null,
    sha256: null,
  };
}

/**
 * useChunkedUpload – the central hook driving the three-stage upload flow.
 *
 * Usage:
 *   const { state, start, pause, resume, reset } = useChunkedUpload();
 */
export function useChunkedUpload() {
  const [state, setState] = useState<UploadState>(makeInitialState);

  // Mutable refs that don't trigger re-renders
  const pausedRef = useRef(false);
  const abortRef = useRef(false);
  const speedSamples = useRef<number[]>([]);

  // ── Helpers ──────────────────────────────────────────────────────────────

  const patch = useCallback((patch: Partial<UploadState>) => {
    setState(prev => ({ ...prev, ...patch }));
  }, []);

  const recordSpeed = useCallback((bytes: number, ms: number) => {
    const bps = bytes / (ms / 1000);
    speedSamples.current.push(bps);
    if (speedSamples.current.length > ROLLING_WINDOW) {
      speedSamples.current.shift();
    }
    const avg = speedSamples.current.reduce((a, b) => a + b, 0) / speedSamples.current.length;
    return avg;
  }, []);

  // ── Start (new upload) ───────────────────────────────────────────────────

  const start = useCallback(async (file: File) => {
    pausedRef.current = false;
    abortRef.current = false;
    speedSamples.current = [];

    setState({
      ...makeInitialState(),
      file,
      phase: 'hashing',
    });

    let sha256 = '';
    try {
      sha256 = await sha256File(file, (_pct) => {
        // Could surface hashing progress if needed
      });
    } catch {
      // Non-fatal – proceed without hash
    }

    patch({ phase: 'initializing', sha256 });

    let uploadId: string;
    let totalChunks: number;
    let chunkSize: number;

    try {
      const init = await initUpload({
        fileName: file.name,
        fileSize: file.size,
        mimeType: file.type || 'application/octet-stream',
        sha256Hash: sha256,
      });
      uploadId = init.upload_id;
      totalChunks = init.total_chunks;
      chunkSize = init.chunk_size;
    } catch (err) {
      patch({ phase: 'error', errorMessage: (err as Error).message });
      return;
    }

    patch({
      phase: 'uploading',
      uploadId,
      totalChunks,
      chunkSize,
      uploadedChunks: new Set<number>(),
      currentChunkIndex: 0,
      bytesSent: 0,
    });

    await runUploadLoop(file, uploadId, chunkSize, totalChunks, new Set<number>(), 0);
  }, [patch, recordSpeed]); // eslint-disable-line react-hooks/exhaustive-deps

  // ── Pause ────────────────────────────────────────────────────────────────

  const pause = useCallback(() => {
    pausedRef.current = true;
    patch({ phase: 'paused' });
  }, [patch]);

  // ── Resume ───────────────────────────────────────────────────────────────

  const resume = useCallback(async () => {
    pausedRef.current = false;

    setState(prev => {
      if (!prev.file || !prev.uploadId) return prev;

      // Fire the async loop without blocking state update
      void (async () => {
        // Re-sync with server to pick up any chunks the server already has
        try {
          const status = await getUploadStatus(prev.uploadId!);
          const serverChunks = new Set<number>(status.uploaded_chunks);
          const nextIndex = status.next_chunk_index;

          patch({
            phase: 'uploading',
            uploadedChunks: serverChunks,
            currentChunkIndex: nextIndex,
            bytesSent: serverChunks.size * prev.chunkSize,
          });

          await runUploadLoop(
            prev.file!,
            prev.uploadId!,
            prev.chunkSize,
            prev.totalChunks,
            serverChunks,
            nextIndex,
          );
        } catch {
          patch({ phase: 'uploading' });
          await runUploadLoop(
            prev.file!,
            prev.uploadId!,
            prev.chunkSize,
            prev.totalChunks,
            prev.uploadedChunks,
            prev.currentChunkIndex,
          );
        }
      })();

      return { ...prev, phase: 'uploading' as const };
    });
  }, [patch]); // eslint-disable-line react-hooks/exhaustive-deps

  // ── Reset ────────────────────────────────────────────────────────────────

  const reset = useCallback(() => {
    abortRef.current = true;
    pausedRef.current = false;
    setState(makeInitialState());
    speedSamples.current = [];
  }, []);

  // ── Core upload loop ─────────────────────────────────────────────────────

  async function runUploadLoop(
    file: File,
    uploadId: string,
    chunkSize: number,
    totalChunks: number,
    alreadyUploaded: Set<number>,
    startIndex: number,
  ) {
    const uploaded = new Set<number>(alreadyUploaded);

    for (let i = startIndex; i < totalChunks; i++) {
      if (abortRef.current) return;

      // Wait while paused
      while (pausedRef.current) {
        await sleep(200);
        if (abortRef.current) return;
      }

      if (uploaded.has(i)) continue; // already done (idempotent)

      const start = i * chunkSize;
      const end = Math.min(start + chunkSize, file.size);
      const chunk = file.slice(start, end);

      const t0 = performance.now();
      let chunkUploaded = false;
      let attempts = 0;
      const maxRetries = 3;

      while (!chunkUploaded && attempts < maxRetries) {
        if (abortRef.current) return;
        while (pausedRef.current) {
          await sleep(200);
          if (abortRef.current) return;
        }

        try {
          await uploadChunk(uploadId, i, chunk);
          chunkUploaded = true;
        } catch (err) {
          attempts++;
          if (attempts >= maxRetries) {
            if (abortRef.current) return;
            patch({
              phase: 'error',
              errorMessage: `Chunk ${i} failed after ${maxRetries} attempts: ${(err as Error).message}`,
            });
            return;
          }
          await sleep(1000 * attempts);
        }
      }
      const elapsed = performance.now() - t0;

      uploaded.add(i);
      const avgSpeed = recordSpeed(chunk.size, elapsed);

      patch({
        uploadedChunks: new Set(uploaded),
        currentChunkIndex: i + 1,
        bytesSent: uploaded.size * chunkSize,
        speedBps: avgSpeed,
      });
    }

    if (abortRef.current) return;

    // All chunks sent – finalise
    patch({ phase: 'completing' });
    try {
      const result = await completeUpload(uploadId);
      patch({ phase: 'done', location: result.location });
    } catch (err) {
      patch({ phase: 'error', errorMessage: (err as Error).message });
    }
  }

  return { state, start, pause, resume, reset };
}

function sleep(ms: number) {
  return new Promise(r => setTimeout(r, ms));
}

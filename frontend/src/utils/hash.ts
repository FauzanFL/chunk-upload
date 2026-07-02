/**
 * Computes the SHA-256 hash of a File using the native Web Crypto API.
 * Reports progress via the optional callback (0–100).
 */
export async function sha256File(
  file: File,
  onProgress?: (percent: number) => void,
): Promise<string> {
  const CHUNK = 4 * 1024 * 1024; // 4 MB read-at-a-time
  const reader = file.stream().getReader();
  const digestStream = crypto.subtle;

  // We need to hash incrementally – use a full-file approach via arrayBuffer
  // for files < 2 GB; for larger files we'd need a streaming digest library.
  // Web Crypto doesn't expose streaming SHA-256, so we use the full buffer here.
  const buffer = await readFileWithProgress(file, CHUNK, onProgress);
  const hashBuffer = await digestStream.digest('SHA-256', buffer);

  // Convert ArrayBuffer to hex string
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');

  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  void reader; // used only to keep the type-checker happy
}

async function readFileWithProgress(
  file: File,
  _chunkSize: number,
  onProgress?: (percent: number) => void,
): Promise<ArrayBuffer> {
  return new Promise((resolve, reject) => {
    const fr = new FileReader();
    fr.onprogress = (e) => {
      if (e.lengthComputable && onProgress) {
        onProgress(Math.round((e.loaded / e.total) * 100));
      }
    };
    fr.onload = () => resolve(fr.result as ArrayBuffer);
    fr.onerror = () => reject(fr.error);
    fr.readAsArrayBuffer(file);
  });
}

/** Format bytes to a human-readable string. */
export function formatBytes(bytes: number, decimals = 1): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`;
}

/** Format bytes/s to human-readable speed. */
export function formatSpeed(bps: number): string {
  return `${formatBytes(bps)}/s`;
}

/** Estimate remaining time. */
export function etaSeconds(remaining: number, bps: number): number {
  if (bps <= 0) return Infinity;
  return remaining / bps;
}

export function formatDuration(seconds: number): string {
  if (!isFinite(seconds) || seconds > 86400) return '—';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

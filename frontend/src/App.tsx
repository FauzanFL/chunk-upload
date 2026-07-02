import { FileDropZone } from './components/FileDropZone';
import { ProgressBar } from './components/ProgressBar';
import { UploadStats } from './components/UploadStats';
import { useChunkedUpload } from './hooks/useChunkedUpload';
import type { UploadPhase } from './types/upload';

const PHASE_LABEL: Record<UploadPhase, string> = {
  idle: 'Select a file to begin',
  hashing: 'Computing SHA-256 checksum…',
  initializing: 'Initialising upload session…',
  uploading: 'Uploading…',
  paused: 'Paused',
  completing: 'Finalising on server…',
  done: 'Upload complete ✓',
  error: 'Upload failed',
};

export default function App() {
  const { state, start, pause, resume, reset } = useChunkedUpload();
  const { phase, uploadedChunks, totalChunks, file } = state;

  const percent = totalChunks > 0 ? (uploadedChunks.size / totalChunks) * 100 : 0;

  const isActive = phase === 'uploading' || phase === 'completing';
  const canPause = phase === 'uploading';
  const canResume = phase === 'paused';
  const canReset = phase !== 'idle' && !isActive;
  const canPickFile = phase === 'idle' || phase === 'done' || phase === 'error';

  return (
    <div className="app">
      <header className="app-header">
        <div className="app-logo">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M12 2L2 7l10 5 10-5-10-5z" />
            <path d="M2 17l10 5 10-5" />
            <path d="M2 12l10 5 10-5" />
          </svg>
        </div>
        <div>
          <h1 className="app-title">ChunkUpload</h1>
          <p className="app-subtitle">Resumable Large File Transfer — Proof of Concept</p>
        </div>
      </header>

      <main className="app-main">
        {/* ── File selector ── */}
        {canPickFile && (
          <FileDropZone onFile={start} disabled={!canPickFile} />
        )}

        {/* ── Active / result state ── */}
        {phase !== 'idle' && (
          <div className="upload-card">
            {/* Status badge */}
            <div className={`phase-badge phase-${phase}`}>
              {phase === 'hashing' || phase === 'initializing' || phase === 'completing' ? (
                <span className="spinner" aria-hidden="true" />
              ) : null}
              {PHASE_LABEL[phase]}
            </div>

            {/* Progress bar (shown once we have chunk data) */}
            {(phase !== 'hashing' && phase !== 'initializing') && (
              <ProgressBar percent={percent} phase={phase} />
            )}

            {/* Stats table */}
            <UploadStats state={state} />

            {/* Success card */}
            {phase === 'done' && state.location && (
              <div className="success-box">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
                  <polyline points="22 4 12 14.01 9 11.01" />
                </svg>
                <div>
                  <p className="success-title">File stored successfully</p>
                  <p className="success-location mono">{state.location}</p>
                </div>
              </div>
            )}

            {/* Error message */}
            {phase === 'error' && state.errorMessage && (
              <div className="error-box">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <circle cx="12" cy="12" r="10" />
                  <line x1="12" y1="8" x2="12" y2="12" />
                  <line x1="12" y1="16" x2="12.01" y2="16" />
                </svg>
                <p>{state.errorMessage}</p>
              </div>
            )}

            {/* Action buttons */}
            <div className="btn-row">
              {canPause && (
                <button className="btn btn-secondary" onClick={pause}>
                  ⏸ Pause
                </button>
              )}
              {canResume && (
                <button className="btn btn-primary" onClick={resume}>
                  ▶ Resume
                </button>
              )}
              {canReset && (
                <button className="btn btn-ghost" onClick={reset}>
                  ↺ New upload
                </button>
              )}
              {phase === 'error' && (
                <button className="btn btn-primary" onClick={() => file && start(file)}>
                  ↻ Retry
                </button>
              )}
            </div>

            {/* Chunk visualiser (mini heatmap) */}
            {totalChunks > 0 && totalChunks <= 200 && (
              <ChunkMap total={totalChunks} uploaded={uploadedChunks} current={state.currentChunkIndex} />
            )}
          </div>
        )}

        {/* ── How it works ── */}
        <HowItWorks />
      </main>

      <footer className="app-footer">
        <span>Backend: Go + Gin · MinIO (S3) · Redis</span>
        <a href="http://localhost:8080/swagger/index.html" target="_blank" rel="noreferrer" className="swagger-link">
          Swagger UI ↗
        </a>
      </footer>
    </div>
  );
}

// ─── Chunk heatmap ───────────────────────────────────────────────────────────

function ChunkMap({ total, uploaded, current }: { total: number; uploaded: Set<number>; current: number }) {
  return (
    <div className="chunk-map" aria-label="Chunk upload map">
      <p className="chunk-map-title">Chunk map ({total} chunks)</p>
      <div className="chunk-grid">
        {Array.from({ length: total }, (_, i) => (
          <div
            key={i}
            className={`chunk-cell ${
              uploaded.has(i) ? 'done' : i === current ? 'active' : 'pending'
            }`}
            title={`Chunk ${i}: ${uploaded.has(i) ? 'done' : i === current ? 'uploading' : 'pending'}`}
          />
        ))}
      </div>
    </div>
  );
}

// ─── How-it-works section ────────────────────────────────────────────────────

function HowItWorks() {
  const steps = [
    {
      n: '01',
      title: 'Initialise',
      body: 'Client hashes the file (SHA-256) and sends metadata. Server returns an Upload ID and ideal chunk size.',
    },
    {
      n: '02',
      title: 'Chunk & Stream',
      body: 'File is sliced via Blob.slice(). Each chunk is POSTed with its index and streamed directly into MinIO multipart.',
    },
    {
      n: '03',
      title: 'Finalise',
      body: 'Server validates all chunks in Redis, calls CompleteMultipartUpload on MinIO, and purges Redis state.',
    },
  ];

  return (
    <section className="how-it-works">
      <h2 className="how-title">Three-stage protocol</h2>
      <div className="how-steps">
        {steps.map(s => (
          <div className="how-step" key={s.n}>
            <span className="how-number">{s.n}</span>
            <div>
              <h3 className="how-step-title">{s.title}</h3>
              <p className="how-step-body">{s.body}</p>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

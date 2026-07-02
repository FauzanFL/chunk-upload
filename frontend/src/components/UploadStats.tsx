import React from 'react';
import type { UploadState } from '../types/upload';
import { etaSeconds, formatBytes, formatDuration, formatSpeed } from '../utils/hash';

interface Props {
  state: UploadState;
}

export const UploadStats: React.FC<Props> = ({ state }) => {
  const { file, uploadedChunks, totalChunks, chunkSize, bytesSent, speedBps, phase } = state;
  if (!file || totalChunks === 0) return null;

  const percent = (uploadedChunks.size / totalChunks) * 100;
  const remaining = file.size - bytesSent;
  const eta = etaSeconds(remaining, speedBps);

  return (
    <div className="stats-grid">
      <StatItem label="File" value={file.name} mono={false} />
      <StatItem label="Size" value={formatBytes(file.size)} />
      <StatItem label="Sent" value={formatBytes(bytesSent)} />
      <StatItem label="Progress" value={`${percent.toFixed(1)}%`} />
      <StatItem label="Chunks" value={`${uploadedChunks.size} / ${totalChunks}`} />
      <StatItem label="Chunk size" value={formatBytes(chunkSize)} />
      {phase === 'uploading' && (
        <>
          <StatItem label="Speed" value={formatSpeed(speedBps)} />
          <StatItem label="ETA" value={formatDuration(eta)} />
        </>
      )}
      {state.uploadId && (
        <StatItem label="Upload ID" value={state.uploadId} mono />
      )}
    </div>
  );
};

interface StatItemProps {
  label: string;
  value: string;
  mono?: boolean;
}

const StatItem: React.FC<StatItemProps> = ({ label, value, mono = true }) => (
  <div className="stat-item">
    <span className="stat-label">{label}</span>
    <span className={`stat-value${mono ? ' mono' : ''}`}>{value}</span>
  </div>
);

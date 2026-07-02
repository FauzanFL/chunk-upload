import React from 'react';

interface Props {
  percent: number;   // 0–100
  phase: string;
}

export const ProgressBar: React.FC<Props> = ({ percent, phase }) => {
  const clamped = Math.min(100, Math.max(0, percent));

  const barColor =
    phase === 'error' ? 'var(--color-error)' :
    phase === 'done'  ? 'var(--color-success)' :
    phase === 'paused' ? 'var(--color-paused)' :
    'var(--color-accent)';

  return (
    <div className="progress-track" role="progressbar" aria-valuenow={clamped} aria-valuemin={0} aria-valuemax={100}>
      <div
        className="progress-fill"
        style={{
          width: `${clamped}%`,
          backgroundColor: barColor,
          transition: phase === 'paused' ? 'none' : 'width 0.3s ease',
        }}
      />
      <span className="progress-label">{clamped.toFixed(1)}%</span>
    </div>
  );
};

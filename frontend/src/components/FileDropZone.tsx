import React, { useCallback, useRef, useState } from 'react';
import { formatBytes } from '../utils/hash';

interface Props {
  onFile: (file: File) => void;
  disabled?: boolean;
}

export const FileDropZone: React.FC<Props> = ({ onFile, disabled = false }) => {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);

  const handleFile = useCallback((file: File) => {
    onFile(file);
  }, [onFile]);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (disabled) return;
    const file = e.dataTransfer.files[0];
    if (file) handleFile(file);
  }, [disabled, handleFile]);

  const onDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    if (!disabled) setDragging(true);
  };

  const onDragLeave = () => setDragging(false);

  const onInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) handleFile(file);
    e.target.value = '';
  };

  return (
    <div
      className={`dropzone${dragging ? ' dragging' : ''}${disabled ? ' disabled' : ''}`}
      onDrop={onDrop}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onClick={() => !disabled && inputRef.current?.click()}
      role="button"
      tabIndex={disabled ? -1 : 0}
      aria-label="Drop a file here or click to select"
      onKeyDown={(e) => e.key === 'Enter' && !disabled && inputRef.current?.click()}
    >
      <input
        ref={inputRef}
        type="file"
        style={{ display: 'none' }}
        onChange={onInputChange}
        aria-hidden="true"
      />
      <div className="dropzone-icon">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="17 8 12 3 7 8" />
          <line x1="12" y1="3" x2="12" y2="15" />
        </svg>
      </div>
      <p className="dropzone-primary">
        {dragging ? 'Drop it!' : 'Drop file here or click to browse'}
      </p>
      <p className="dropzone-secondary">
        Supports video, audio, images, PDF, ZIP · Max 10 GB
      </p>
      <p className="dropzone-secondary">
        Max chunk size: {formatBytes(5 * 1024 * 1024)}
      </p>
    </div>
  );
};

"use client";

import { useRef } from "react";
import { ImageIcon } from "lucide-react";

export const MAX_FILE_BYTES = 10 * 1024 * 1024;

interface FileDropZoneProps {
  readonly file: File | null;
  readonly preview: string | null;
  readonly onFileSelect: (file: File, preview: string | null) => void;
  readonly onError: (msg: string) => void;
  readonly tooLargeMsg: string;
  readonly clickLabel: string;
  readonly dragLabel?: string;
  readonly typesLabel: string;
  readonly filePrefix: string;
}

export function FileDropZone({
  file,
  preview,
  onFileSelect,
  onError,
  tooLargeMsg,
  clickLabel,
  dragLabel,
  typesLabel,
  filePrefix,
}: FileDropZoneProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);

  function handleFile(f: File) {
    if (f.size > MAX_FILE_BYTES) {
      onError(tooLargeMsg);
      return;
    }
    if (f.type.startsWith("image/")) {
      const reader = new FileReader();
      reader.onload = () => onFileSelect(f, reader.result as string);
      reader.readAsDataURL(f);
    } else {
      onFileSelect(f, null);
    }
  }

  return (
    <>
      <button
        type="button"
        className="w-full border-2 border-dashed border-white/20 rounded-xl p-6 text-center cursor-pointer hover:border-gold-400/50 hover:bg-white/[0.02] transition-colors"
        onClick={() => fileInputRef.current?.click()}
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault();
          const f = e.dataTransfer.files?.[0];
          if (f) handleFile(f);
        }}
      >
        {preview ? (
          <img
            src={preview}
            alt="Vista previa"
            className="max-h-48 mx-auto rounded-lg object-contain"
          />
        ) : (
          <div className="flex flex-col items-center gap-3 text-text-muted">
            <ImageIcon className="w-10 h-10 opacity-40" />
            <div>
              <p className="text-sm font-medium text-text-secondary">
                {clickLabel}
              </p>
              {dragLabel && <p className="text-xs mt-0.5">{dragLabel}</p>}
              <p className="text-xs mt-1 opacity-60">{typesLabel}</p>
            </div>
          </div>
        )}
      </button>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/*,application/pdf"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) handleFile(f);
        }}
      />

      {file && (
        <p className="text-xs text-text-muted">
          {filePrefix}{" "}
          <span className="text-text-secondary">{file.name}</span> (
          {(file.size / 1024).toFixed(0)} KB)
        </p>
      )}
    </>
  );
}

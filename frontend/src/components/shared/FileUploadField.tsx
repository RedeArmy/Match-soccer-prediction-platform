import { CheckCircle, Upload, X, Clock } from "lucide-react";

function getBorderClass(hasSelectedFile: boolean, isPending: boolean): string {
  if (!hasSelectedFile) return "border-blue-600 hover:border-gold-400";
  return isPending
    ? "border-amber-500 bg-amber-500/10 hover:border-amber-400"
    : "border-green-500 bg-green-500/10 hover:border-green-400";
}

interface FileUploadFieldProps {
  readonly label: string;
  readonly accept?: string;
  readonly fileName?: string;
  readonly hasFile?: boolean;
  readonly isPending?: boolean;
  readonly pendingLabel?: string;
  readonly previewUrl?: string;
  readonly onRemove?: () => void;
  readonly onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

export function FileUploadField({
  label,
  accept = "image/jpeg,image/png,image/webp,application/pdf",
  fileName,
  hasFile = false,
  isPending = false,
  pendingLabel = "{pendingLabel}",
  previewUrl,
  onRemove,
  onChange,
}: FileUploadFieldProps) {
  const hasSelectedFile = hasFile || Boolean(fileName);
  const borderClass = getBorderClass(hasSelectedFile, isPending);

  return (
    <label
      className={`relative flex flex-col border-2 border-dashed rounded-xl cursor-pointer transition-colors overflow-hidden ${borderClass}`}
    >
      {previewUrl ? (
        <div className="w-full">
          {/* eslint-disable-next-line @next/next/no-img-element -- previewUrl is a blob: object URL, incompatible with next/image */}
          <img
            src={previewUrl}
            alt={fileName ?? label}
            className="w-full h-36 object-cover"
          />
          <div
            className={`flex items-center gap-2 px-3 py-2 ${isPending ? "bg-amber-500/10" : "bg-green-500/10"}`}
          >
            {isPending ? (
              <Clock className="w-4 h-4 shrink-0 text-amber-400" />
            ) : (
              <CheckCircle className="w-4 h-4 shrink-0 text-green-400" />
            )}
            <div className="min-w-0">
              <p
                className={`text-xs truncate ${isPending ? "text-amber-300" : "text-green-300"}`}
              >
                {fileName}
              </p>
              {isPending && (
                <p className="text-[10px] text-amber-500">{pendingLabel}</p>
              )}
            </div>
          </div>
        </div>
      ) : (
        <div className="flex flex-col items-center gap-2 p-4 text-center">
          {!hasSelectedFile && (
            <>
              <Upload className="w-6 h-6 text-blue-400" />
              <p className="text-sm text-text-secondary">{label}</p>
            </>
          )}
          {hasSelectedFile && isPending && (
            <>
              <Clock className="w-6 h-6 text-amber-400" />
              <p className="text-sm text-amber-300">{fileName}</p>
              <p className="text-[10px] text-amber-500">{pendingLabel}</p>
            </>
          )}
          {hasSelectedFile && !isPending && (
            <>
              <CheckCircle className="w-6 h-6 text-green-400" />
              <p className="text-sm text-green-300">{fileName}</p>
            </>
          )}
        </div>
      )}
      <input
        type="file"
        accept={accept}
        onChange={onChange}
        className="sr-only"
      />
      {hasSelectedFile && onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onRemove();
          }}
          className="absolute top-2 right-2 rounded-full p-0.5 bg-black/50 text-white hover:bg-black/70 transition-colors"
          aria-label="Eliminar archivo"
        >
          <X className="w-4 h-4" />
        </button>
      )}
    </label>
  );
}

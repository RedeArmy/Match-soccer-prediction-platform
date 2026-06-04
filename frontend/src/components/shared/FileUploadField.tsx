import { Upload } from "lucide-react";

interface FileUploadFieldProps {
  label: string;
  accept?: string;
  fileName?: string;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

export function FileUploadField({
  label,
  accept = "image/jpeg,image/png,image/webp,application/pdf",
  fileName,
  onChange,
}: FileUploadFieldProps) {
  return (
    <label className="flex flex-col items-center gap-2 border-2 border-dashed border-blue-600 rounded-xl p-4 cursor-pointer hover:border-gold-400 transition-colors text-center">
      <Upload className="w-6 h-6 text-blue-400" />
      <span className="text-sm text-text-secondary">{fileName ?? label}</span>
      <input
        type="file"
        accept={accept}
        onChange={onChange}
        className="sr-only"
      />
    </label>
  );
}

import { cn } from "@/lib/utils";

interface LoadingStateProps {
  readonly className?: string;
  readonly rows?: number;
}

export function LoadingState({ className, rows = 3 }: LoadingStateProps) {
  return (
    <div className={cn("space-y-3 animate-pulse", className)}>
      {Array.from({ length: rows }, (_, i) => `skeleton-${i}`).map((key) => (
        <div key={key} className="h-16 rounded-xl bg-blue-800/50" />
      ))}
    </div>
  );
}

export function LoadingSpinner({
  size = 20,
  className,
}: Readonly<{ size?: number; className?: string }>) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      className={cn("animate-spin text-gold-400", className)}
      aria-label="Cargando"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8v8H4z"
      />
    </svg>
  );
}

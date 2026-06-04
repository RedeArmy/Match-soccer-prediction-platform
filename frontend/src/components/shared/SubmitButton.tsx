import { LoadingSpinner } from "@/components/shared/LoadingState";

interface SubmitButtonProps {
  readonly isPending: boolean;
  readonly disabled?: boolean;
  readonly onClick?: () => void;
  readonly children: React.ReactNode;
}

export function SubmitButton({
  isPending,
  disabled,
  onClick,
  children,
}: SubmitButtonProps) {
  return (
    <button
      onClick={onClick}
      disabled={isPending || disabled}
      className="btn-gold w-full flex items-center justify-center gap-2"
    >
      {isPending ? <LoadingSpinner size={18} /> : children}
    </button>
  );
}

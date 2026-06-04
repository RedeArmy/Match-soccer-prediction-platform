import { LoadingSpinner } from "@/components/shared/LoadingState";

interface SubmitButtonProps {
  isPending: boolean;
  disabled?: boolean;
  onClick?: () => void;
  children: React.ReactNode;
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

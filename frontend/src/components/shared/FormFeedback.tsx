interface FormFeedbackProps {
  readonly error?: string;
  readonly success?: string;
  readonly center?: boolean;
}

export function FormFeedback({ error, success, center }: FormFeedbackProps) {
  if (!error && !success) return null;
  const align = center ? " text-center" : "";
  return (
    <div>
      {error && <p className={`text-red-400 text-sm${align}`}>{error}</p>}
      {success && <p className={`text-green-400 text-sm${align}`}>{success}</p>}
    </div>
  );
}

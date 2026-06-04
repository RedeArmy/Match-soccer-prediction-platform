interface FormFieldProps {
  readonly label: string;
  readonly spacing?: "tight" | "normal";
  readonly children: React.ReactNode;
}

export function FormField({
  label,
  spacing = "tight",
  children,
}: FormFieldProps) {
  const mb = spacing === "tight" ? "mb-1" : "mb-1.5";
  return (
    <div>
      <label className={`block text-sm text-text-secondary ${mb}`}>
        {label}
      </label>
      {children}
    </div>
  );
}

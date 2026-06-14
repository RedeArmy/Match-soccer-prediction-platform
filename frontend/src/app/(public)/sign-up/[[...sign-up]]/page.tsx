import { SignUp } from "@clerk/nextjs";

export default function SignUpPage() {
  return (
    <div className="min-h-screen bg-[var(--bg-base)] flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="font-display text-5xl text-gold-400 mb-2">
            ÚNETE GRATIS
          </h1>
          <p className="text-text-secondary">
            Crea tu cuenta en segundos y empieza a predecir
          </p>
        </div>
        <SignUp routing="hash" />
      </div>
    </div>
  );
}

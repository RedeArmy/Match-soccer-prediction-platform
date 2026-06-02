import { SignIn } from '@clerk/nextjs'

export default function SignInPage() {
  return (
    <div className="min-h-screen bg-[var(--bg-base)] flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="font-display text-5xl text-gold-400 mb-2">BIENVENIDO</h1>
          <p className="text-text-secondary">Ingresa a tu cuenta para continuar</p>
        </div>
        <SignIn routing="hash" />
      </div>
    </div>
  )
}

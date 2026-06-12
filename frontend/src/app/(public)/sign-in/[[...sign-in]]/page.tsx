import { SignIn } from '@clerk/nextjs'

export default function SignInPage() {
  return (
    <div className="min-h-screen bg-[var(--bg-base)] flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="font-display text-5xl text-gold-400 mb-5">BIENVENIDO</h1>

          {/* K spinner — spinning arc + brand letter */}
          <div className="relative w-12 h-12 mx-auto">
            <div className="absolute inset-0 rounded-full border-2 border-gold-600/25 border-t-gold-400 animate-spin [animation-duration:1.2s]" />
            <div className="absolute inset-0 flex items-center justify-center">
              <span className="font-display text-lg text-gold-400 leading-none select-none">K</span>
            </div>
          </div>
        </div>
        <SignIn routing="hash" />
      </div>
    </div>
  )
}

import Link from 'next/link'

export function Footer() {
  return (
    <footer className="border-t border-blue-800/60 bg-blue-950 mt-auto">
      <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8 py-8">
        <div className="flex flex-col md:flex-row items-center justify-between gap-4">

          <div className="flex items-center gap-2">
            <span className="font-display text-2xl text-gold-400">QM</span>
            <span className="text-text-muted text-sm">Mundial 2026</span>
          </div>

          <nav className="flex flex-wrap justify-center gap-x-6 gap-y-2 text-sm text-text-muted">
            <Link href="/tournaments" className="hover:text-text-secondary transition-colors">Torneos</Link>
            <Link href="/sign-in"    className="hover:text-text-secondary transition-colors">Iniciar sesión</Link>
            <Link href="/sign-up"    className="hover:text-text-secondary transition-colors">Registrarse</Link>
          </nav>

          <p className="text-xs text-text-muted text-center md:text-right max-w-xs">
            Juega responsablemente. Solo para mayores de 18 años.
          </p>
        </div>
      </div>
    </footer>
  )
}

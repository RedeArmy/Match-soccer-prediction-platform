import type { Metadata } from 'next'
import { ClerkProvider } from '@clerk/nextjs'
import { Inter } from 'next/font/google'
import { Providers } from './providers'
import '@/styles/globals.css'

const inter = Inter({
  subsets: ['latin'],
  variable: '--font-inter',
  display: 'swap',
})

export const metadata: Metadata = {
  title:       'Quiniela Mundial 2026',
  description: 'Predice los resultados del Mundial de Fútbol 2026 y compite por premios',
  metadataBase: new URL(process.env.NEXT_PUBLIC_APP_URL ?? 'http://localhost:3000'),
  openGraph: {
    type:   'website',
    locale: 'es_GT',
    title:  'Quiniela Mundial 2026',
  },
}

const clerkAppearance = {
  variables: {
    colorPrimary:         '#D4A017',
    colorBackground:      '#0D1F3C',
    colorText:            '#F0F4FA',
    colorTextSecondary:   '#A8B8CC',
    colorInputBackground: '#112850',
    colorInputText:       '#F0F4FA',
    colorNeutral:         '#A8B8CC',
    borderRadius:         '8px',
    fontFamily:           'var(--font-inter)',
  },
  elements: {
    card:                   'shadow-2xl border border-blue-600/50 bg-blue-900',
    formButtonPrimary:      'bg-gradient-to-r from-gold-600 to-gold-300 text-blue-950 font-bold hover:opacity-90',
    footerActionLink:       'text-gold-400 hover:text-gold-300',
    formFieldInput:         'bg-blue-700 border-blue-600 text-white placeholder:text-blue-400/60',
    identityPreviewText:    'text-white',
    headerTitle:            'text-white font-display',
    socialButtonsBlockButton: 'bg-blue-700 border-blue-600 text-white hover:bg-blue-600',
    dividerLine:            'bg-blue-600',
    dividerText:            'text-blue-400',
  },
}

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <ClerkProvider appearance={clerkAppearance}>
      <html lang="es" className={inter.variable} suppressHydrationWarning>
        <head>
          <link rel="preconnect" href="https://fonts.googleapis.com" />
          <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
          {/* eslint-disable-next-line @next/next/no-page-custom-font -- App Router layout is the correct place for global fonts */}
          <link
            href="https://fonts.googleapis.com/css2?family=Bebas+Neue&family=JetBrains+Mono:wght@400;500&display=swap"
            rel="stylesheet"
          />
          <style>{`
            :root {
              --font-bebas: 'Bebas Neue';
              --font-jetbrains: 'JetBrains Mono';
            }
          `}</style>
        </head>
        <body>
          <Providers>{children}</Providers>
        </body>
      </html>
    </ClerkProvider>
  )
}

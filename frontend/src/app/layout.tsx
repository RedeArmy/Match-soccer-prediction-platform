import type { Metadata } from 'next'
import { headers } from 'next/headers'
import { ClerkProvider } from '@clerk/nextjs'
import { Providers } from './providers'
import '@/styles/globals.css'

export const metadata: Metadata = {
  title: 'Kiniela',
  description: 'Predice resultados del Mundial 2026 y compite en kinielas internacionales',
  metadataBase: new URL(process.env.NEXT_PUBLIC_APP_URL ?? 'http://localhost:3000'),
  openGraph: {
    type: 'website',
    locale: 'es_GT',
    title: 'Kiniela',
  },
}

const clerkAppearance = {
  variables: {
    colorPrimary: '#D4A017',
    colorBackground: '#0D1420',
    colorText: '#F4F7FB',
    colorTextSecondary: '#B5C2D1',
    colorInputBackground: '#16243A',
    colorInputText: '#F4F7FB',
    colorNeutral: '#B5C2D1',
    borderRadius: '8px',
    fontFamily: 'var(--font-inter)',
  },
  elements: {
    card: 'shadow-2xl border border-white/10 bg-[#0D1420]',
    formButtonPrimary: 'bg-gradient-to-r from-gold-600 to-gold-300 text-blue-950 font-bold hover:opacity-90',
    footerActionLink: 'text-gold-400 hover:text-gold-300',
    formFieldInput: 'bg-blue-700 border-blue-600 text-white placeholder:text-blue-400/60',
    identityPreviewText: 'text-white',
    headerTitle: 'text-white font-display',
    socialButtonsBlockButton: 'bg-blue-700 border-blue-600 text-white hover:bg-blue-600',
    dividerLine: 'bg-blue-600',
    dividerText: 'text-blue-400',
  },
}

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const nonce = (await headers()).get('x-nonce') ?? undefined
  return (
    <ClerkProvider appearance={clerkAppearance} nonce={nonce}>
      <html lang="es" suppressHydrationWarning>
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
              --font-inter: Inter, ui-sans-serif, system-ui, sans-serif;
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

'use client'

import { createContext, useContext, useEffect, useMemo, useState } from 'react'

export type Locale = 'es' | 'en'

interface TranslationRecord {
  [key: string]: TranslationValue
}

type TranslationValue = string | TranslationRecord
type TranslationTree = Record<string, TranslationValue>

const dictionaries = {
  es: {
    common: {
      brand: 'Quiniela 26',
      event: 'Mundial 2026',
      tournaments: 'Torneos',
      dashboard: 'Dashboard',
      balance: 'Balance',
      predictions: 'Predicciones',
      signIn: 'Iniciar sesion',
      signUp: 'Registrarse',
      signOut: 'Cerrar sesion',
      language: 'Idioma',
      openMenu: 'Abrir menu',
      closeMenu: 'Cerrar menu',
      viewAll: 'Ver todo',
      explore: 'Explorar',
      loading: 'Cargando',
      save: 'Guardar',
      saving: 'Guardando',
      error: 'Error',
      stale: 'desactualizado',
      updated: 'Actualizado',
      responsible: 'Juega responsablemente. Solo para mayores de 18 anos.',
    },
    status: {
      active: 'Activo',
      upcoming: 'Proximo',
      ended: 'Finalizado',
      pending: 'Pendiente',
      approved: 'Aprobado',
      rejected: 'Rechazado',
      connected: 'Conectado',
      reconnecting: 'Reconectando',
      failed: 'Error',
      in_progress: 'En curso',
      open: 'Abierto',
      finished: 'Finalizado',
      scheduled: 'Programado',
      cancelled: 'Cancelado',
      unverified: 'Sin verificar',
      submitted: 'Enviado',
      under_review: 'En revision',
    },
    nav: {
      home: 'Inicio',
      profile: 'Perfil',
      matches: 'Partidos',
    },
    landing: {
      eyebrow: 'Quiniela internacional',
      title: 'Quiniela Mundial 2026',
      subtitle: 'Predice marcadores, administra tus quinielas y compite en una experiencia moderna inspirada en Canada, Mexico y Estados Unidos.',
      primary: 'Entrar al tablero',
      secondary: 'Ver torneos',
      rate: 'Tipo de cambio',
      buy: 'compra',
      howTitle: 'Flujo de juego',
      step1Title: 'Unete',
      step1Desc: 'Crea tu cuenta, verifica tu perfil y entra a una quiniela.',
      step2Title: 'Predice',
      step2Desc: 'Registra marcadores antes del silbatazo inicial de cada partido.',
      step3Title: 'Compite',
      step3Desc: 'Suma puntos, consulta rankings y cobra premios en GTQ.',
      opsTitle: 'Operaciones listas para crecer',
      opsCopy: 'Pagos, KYC, notificaciones, rankings y observabilidad viven en un producto compacto y preparado para torneos internacionales.',
    },
    dashboard: {
      hello: 'Hola',
      player: 'Jugador',
      subtitle: 'Centro de control personal',
      kycTitle: 'Verifica tu identidad',
      kycCopy: 'Completa tu KYC para poder hacer retiros.',
      kycAction: 'Verificar ahora',
      myPools: 'Mis quinielas',
      exploreTournaments: 'Explorar torneos',
      noPools: 'Aun no tienes quinielas',
      noPoolsDesc: 'Unete a un torneo para empezar a predecir',
      recentTransactions: 'Ultimas transacciones',
      noTransactions: 'Sin transacciones aun',
      participants: 'participantes',
      commandTitle: 'Vista de torneo',
      commandCopy: 'Gestiona balance, quinielas y predicciones desde un panel compacto.',
    },
    balanceCard: {
      title: 'Balance',
      available: 'Disponible',
      reserved: 'Reservado',
      pending: 'Pendiente',
      deposit: 'Depositar',
      withdraw: 'Retirar',
    },
    tournaments: {
      title: 'Torneos',
      subtitle: 'Explora quinielas disponibles y revisa el estado de tus grupos.',
      emptyTitle: 'No hay torneos disponibles',
      emptyDesc: 'Vuelve pronto para ver las proximas quinielas',
      prize: 'Pozo',
      entry: 'Inscripcion',
      enter: 'Entrar',
      details: 'Ver detalles',
      members: 'miembros',
    },
    predictions: {
      title: 'Panel de predicciones',
      subtitle: 'Agrega o actualiza marcadores antes de que inicie cada partido.',
      filterAll: 'Todos',
      filterPending: 'Pendientes',
      filterSaved: 'Guardados',
      noMatches: 'No hay partidos programados',
      noMatchesDesc: 'Cuando el calendario este disponible, podras capturar tus marcadores aqui.',
      kickoff: 'Inicio',
      phase: 'Fase',
      venue: 'Sede',
      saved: 'Guardado',
      unsaved: 'Sin guardar',
      locked: 'Bloqueado',
      score: 'Marcador',
      home: 'Local',
      away: 'Visitante',
      submit: 'Guardar prediccion',
      update: 'Actualizar',
      success: 'Prediccion guardada',
      error: 'No se pudo guardar la prediccion',
      exactHint: 'Los marcadores se cierran al iniciar el partido.',
      points: 'pts',
    },
    ticker: {
      live: 'Tipo de cambio en tiempo real',
      outdated: 'tipo de cambio desactualizado',
    },
  },
  en: {
    common: {
      brand: 'Quiniela 26',
      event: 'World Cup 2026',
      tournaments: 'Tournaments',
      dashboard: 'Dashboard',
      balance: 'Balance',
      predictions: 'Predictions',
      signIn: 'Sign in',
      signUp: 'Create account',
      signOut: 'Sign out',
      language: 'Language',
      openMenu: 'Open menu',
      closeMenu: 'Close menu',
      viewAll: 'View all',
      explore: 'Explore',
      loading: 'Loading',
      save: 'Save',
      saving: 'Saving',
      error: 'Error',
      stale: 'stale',
      updated: 'Updated',
      responsible: 'Play responsibly. Adults 18+ only.',
    },
    status: {
      active: 'Active',
      upcoming: 'Upcoming',
      ended: 'Ended',
      pending: 'Pending',
      approved: 'Approved',
      rejected: 'Rejected',
      connected: 'Connected',
      reconnecting: 'Reconnecting',
      failed: 'Error',
      in_progress: 'In progress',
      open: 'Open',
      finished: 'Finished',
      scheduled: 'Scheduled',
      cancelled: 'Cancelled',
      unverified: 'Unverified',
      submitted: 'Submitted',
      under_review: 'Under review',
    },
    nav: {
      home: 'Home',
      profile: 'Profile',
      matches: 'Matches',
    },
    landing: {
      eyebrow: 'International football pool',
      title: 'World Cup 2026 Quiniela',
      subtitle: 'Predict scores, manage your pools, and compete in a modern experience inspired by Canada, Mexico, and the United States.',
      primary: 'Open dashboard',
      secondary: 'View tournaments',
      rate: 'Exchange rate',
      buy: 'buy',
      howTitle: 'Game flow',
      step1Title: 'Join',
      step1Desc: 'Create your account, verify your profile, and enter a pool.',
      step2Title: 'Predict',
      step2Desc: 'Submit scores before kickoff for every match.',
      step3Title: 'Compete',
      step3Desc: 'Earn points, check rankings, and collect prizes in GTQ.',
      opsTitle: 'Built for scale',
      opsCopy: 'Payments, KYC, notifications, rankings, and observability live in a compact product ready for international tournaments.',
    },
    dashboard: {
      hello: 'Hello',
      player: 'Player',
      subtitle: 'Personal command center',
      kycTitle: 'Verify your identity',
      kycCopy: 'Complete KYC to unlock withdrawals.',
      kycAction: 'Verify now',
      myPools: 'My pools',
      exploreTournaments: 'Explore tournaments',
      noPools: 'No pools yet',
      noPoolsDesc: 'Join a tournament to start predicting',
      recentTransactions: 'Recent transactions',
      noTransactions: 'No transactions yet',
      participants: 'participants',
      commandTitle: 'Tournament view',
      commandCopy: 'Manage balance, pools, and predictions from a compact workspace.',
    },
    balanceCard: {
      title: 'Balance',
      available: 'Available',
      reserved: 'Reserved',
      pending: 'Pending',
      deposit: 'Deposit',
      withdraw: 'Withdraw',
    },
    tournaments: {
      title: 'Tournaments',
      subtitle: 'Explore available pools and review your group status.',
      emptyTitle: 'No tournaments available',
      emptyDesc: 'Check back soon for upcoming pools',
      prize: 'Prize pool',
      entry: 'Entry',
      enter: 'Enter',
      details: 'View details',
      members: 'members',
    },
    predictions: {
      title: 'Prediction panel',
      subtitle: 'Add or update scores before each match kicks off.',
      filterAll: 'All',
      filterPending: 'Pending',
      filterSaved: 'Saved',
      noMatches: 'No scheduled matches',
      noMatchesDesc: 'When the calendar is available, you will capture your scores here.',
      kickoff: 'Kickoff',
      phase: 'Phase',
      venue: 'Venue',
      saved: 'Saved',
      unsaved: 'Unsaved',
      locked: 'Locked',
      score: 'Score',
      home: 'Home',
      away: 'Away',
      submit: 'Save prediction',
      update: 'Update',
      success: 'Prediction saved',
      error: 'Could not save prediction',
      exactHint: 'Scores lock when the match starts.',
      points: 'pts',
    },
    ticker: {
      live: 'Live exchange rate',
      outdated: 'exchange rate is stale',
    },
  },
} satisfies Record<Locale, TranslationTree>

interface I18nContextValue {
  locale: Locale
  setLocale: (locale: Locale) => void
  t: (key: string) => string
}

const I18nContext = createContext<I18nContextValue | null>(null)

function getNestedValue(tree: TranslationTree, key: string): string | undefined {
  let current: TranslationValue | undefined = tree
  for (const part of key.split('.')) {
    if (!current || typeof current === 'string') return undefined
    current = current[part]
  }
  return typeof current === 'string' ? current : undefined
}

export function I18nProvider({ children }: Readonly<{ children: React.ReactNode }>) {
  const [locale, setLocaleState] = useState<Locale>('es')

  useEffect(() => {
    const stored = window.localStorage.getItem('quiniela-locale')
    if (stored === 'es' || stored === 'en') {
      setLocaleState(stored)
      document.documentElement.lang = stored
    }
  }, [])

  const value = useMemo<I18nContextValue>(() => ({
    locale,
    setLocale: (nextLocale) => {
      setLocaleState(nextLocale)
      window.localStorage.setItem('quiniela-locale', nextLocale)
      document.documentElement.lang = nextLocale
    },
    t: (key) => getNestedValue(dictionaries[locale], key) ?? getNestedValue(dictionaries.es, key) ?? key,
  }), [locale])

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n() {
  const context = useContext(I18nContext)
  if (!context) throw new Error('useI18n must be used within I18nProvider')
  return context
}

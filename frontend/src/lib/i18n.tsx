"use client";

import { createContext, useContext, useEffect, useMemo, useState } from "react";

export type Locale = "es" | "en";

type LeafEntry = Record<Locale, string>;
type TranslationNode = { [key: string]: LeafEntry | TranslationNode };

// prettier-ignore
const translations: TranslationNode = {
  common: {
    brand:       { es: 'Quiniela 26',          en: 'Quiniela 26'          },
    event:       { es: 'Mundial 2026',         en: 'World Cup 2026'       },
    tournaments: { es: 'Torneos',              en: 'Tournaments'          },
    dashboard:   { es: 'Dashboard',            en: 'Dashboard'            },
    balance:     { es: 'Balance',              en: 'Balance'              },
    predictions: { es: 'Predicciones',         en: 'Predictions'          },
    signIn:      { es: 'Iniciar sesion',       en: 'Sign in'              },
    signUp:      { es: 'Registrarse',          en: 'Create account'       },
    signOut:     { es: 'Cerrar sesion',        en: 'Sign out'             },
    language:    { es: 'Idioma',               en: 'Language'             },
    openMenu:    { es: 'Abrir menu',           en: 'Open menu'            },
    closeMenu:   { es: 'Cerrar menu',          en: 'Close menu'           },
    viewAll:     { es: 'Ver todo',             en: 'View all'             },
    explore:     { es: 'Explorar',             en: 'Explore'              },
    loading:     { es: 'Cargando',             en: 'Loading'              },
    save:        { es: 'Guardar',              en: 'Save'                 },
    saving:      { es: 'Guardando',            en: 'Saving'               },
    error:       { es: 'Error',                en: 'Error'                },
    stale:       { es: 'desactualizado',       en: 'stale'                },
    updated:     { es: 'Actualizado',          en: 'Updated'              },
    responsible: { es: 'Juega responsablemente. Solo para mayores de 18 anos.', en: 'Play responsibly. Adults 18+ only.' },
  },
  status: {
    active:       { es: 'Activo',         en: 'Active'       },
    upcoming:     { es: 'Proximo',        en: 'Upcoming'     },
    ended:        { es: 'Finalizado',     en: 'Ended'        },
    pending:      { es: 'Pendiente',      en: 'Pending'      },
    approved:     { es: 'Aprobado',       en: 'Approved'     },
    rejected:     { es: 'Rechazado',      en: 'Rejected'     },
    connected:    { es: 'Conectado',      en: 'Connected'    },
    reconnecting: { es: 'Reconectando',   en: 'Reconnecting' },
    failed:       { es: 'Error',          en: 'Error'        },
    in_progress:  { es: 'En curso',       en: 'In progress'  },
    open:         { es: 'Abierto',        en: 'Open'         },
    finished:     { es: 'Finalizado',     en: 'Finished'     },
    scheduled:    { es: 'Programado',     en: 'Scheduled'    },
    cancelled:    { es: 'Cancelado',      en: 'Cancelled'    },
    unverified:   { es: 'Sin verificar',  en: 'Unverified'   },
    submitted:    { es: 'Enviado',        en: 'Submitted'    },
    under_review: { es: 'En revision',    en: 'Under review' },
  },
  nav: {
    home:    { es: 'Inicio',   en: 'Home'    },
    profile: { es: 'Perfil',   en: 'Profile' },
    matches: { es: 'Partidos', en: 'Matches' },
  },
  landing: {
    eyebrow:   { es: 'Quiniela internacional',     en: 'International football pool'  },
    title:     { es: 'Quiniela Mundial 2026',       en: 'World Cup 2026 Quiniela'      },
    subtitle:  { es: 'Predice marcadores, administra tus quinielas y compite en una experiencia moderna inspirada en Canada, Mexico y Estados Unidos.', en: 'Predict scores, manage your pools, and compete in a modern experience inspired by Canada, Mexico, and the United States.' },
    primary:   { es: 'Entrar al tablero',           en: 'Open dashboard'               },
    secondary: { es: 'Ver torneos',                 en: 'View tournaments'             },
    rate:      { es: 'Tipo de cambio',              en: 'Exchange rate'                },
    buy:       { es: 'compra',                      en: 'buy'                          },
    howTitle:  { es: 'Flujo de juego',              en: 'Game flow'                    },
    step1Title:{ es: 'Unete',                       en: 'Join'                         },
    step1Desc: { es: 'Crea tu cuenta, verifica tu perfil y entra a una quiniela.',     en: 'Create your account, verify your profile, and enter a pool.'      },
    step2Title:{ es: 'Predice',                     en: 'Predict'                      },
    step2Desc: { es: 'Registra marcadores antes del silbatazo inicial de cada partido.', en: 'Submit scores before kickoff for every match.'                  },
    step3Title:{ es: 'Compite',                     en: 'Compete'                      },
    step3Desc: { es: 'Suma puntos, consulta rankings y cobra premios en GTQ.',         en: 'Earn points, check rankings, and collect prizes in GTQ.'          },
    opsTitle:  { es: 'Operaciones listas para crecer', en: 'Built for scale'           },
    opsCopy:   { es: 'Pagos, KYC, notificaciones, rankings y observabilidad viven en un producto compacto y preparado para torneos internacionales.', en: 'Payments, KYC, notifications, rankings, and observability live in a compact product ready for international tournaments.' },
  },
  dashboard: {
    hello:              { es: 'Hola',                          en: 'Hello'                        },
    player:             { es: 'Jugador',                       en: 'Player'                       },
    subtitle:           { es: 'Centro de control personal',    en: 'Personal command center'      },
    kycTitle:           { es: 'Verifica tu identidad',         en: 'Verify your identity'         },
    kycCopy:            { es: 'Completa tu KYC para poder hacer retiros.', en: 'Complete KYC to unlock withdrawals.' },
    kycAction:          { es: 'Verificar ahora',               en: 'Verify now'                   },
    myPools:            { es: 'Mis quinielas',                 en: 'My pools'                     },
    exploreTournaments: { es: 'Explorar torneos',              en: 'Explore tournaments'          },
    noPools:            { es: 'Aun no tienes quinielas',       en: 'No pools yet'                 },
    noPoolsDesc:        { es: 'Unete a un torneo para empezar a predecir', en: 'Join a tournament to start predicting' },
    recentTransactions: { es: 'Ultimas transacciones',         en: 'Recent transactions'          },
    noTransactions:     { es: 'Sin transacciones aun',         en: 'No transactions yet'          },
    participants:       { es: 'participantes',                 en: 'participants'                 },
    commandTitle:       { es: 'Vista de torneo',               en: 'Tournament view'              },
    commandCopy:        { es: 'Gestiona balance, quinielas y predicciones desde un panel compacto.', en: 'Manage balance, pools, and predictions from a compact workspace.' },
  },
  balanceCard: {
    title:    { es: 'Balance',    en: 'Balance'  },
    available:{ es: 'Disponible', en: 'Available'},
    reserved: { es: 'Reservado',  en: 'Reserved' },
    pending:  { es: 'Pendiente',  en: 'Pending'  },
    deposit:  { es: 'Depositar',  en: 'Deposit'  },
    withdraw: { es: 'Retirar',    en: 'Withdraw' },
  },
  tournaments: {
    title:      { es: 'Torneos',                                               en: 'Tournaments'                              },
    subtitle:   { es: 'Explora quinielas disponibles y revisa el estado de tus grupos.', en: 'Explore available pools and review your group status.' },
    emptyTitle: { es: 'No hay torneos disponibles',                            en: 'No tournaments available'                 },
    emptyDesc:  { es: 'Vuelve pronto para ver las proximas quinielas',         en: 'Check back soon for upcoming pools'       },
    prize:      { es: 'Pozo',                                                  en: 'Prize pool'                               },
    entry:      { es: 'Inscripcion',                                           en: 'Entry'                                    },
    enter:      { es: 'Entrar',                                                en: 'Enter'                                    },
    details:    { es: 'Ver detalles',                                          en: 'View details'                             },
    members:    { es: 'miembros',                                              en: 'members'                                  },
  },
  predictions: {
    title:        { es: 'Panel de predicciones',     en: 'Prediction panel'          },
    subtitle:     { es: 'Agrega o actualiza marcadores antes de que inicie cada partido.', en: 'Add or update scores before each match kicks off.' },
    filterAll:    { es: 'Todos',                     en: 'All'                        },
    filterPending:{ es: 'Pendientes',                en: 'Pending'                    },
    filterSaved:  { es: 'Guardados',                 en: 'Saved'                      },
    noMatches:    { es: 'No hay partidos programados', en: 'No scheduled matches'     },
    noMatchesDesc:{ es: 'Cuando el calendario este disponible, podras capturar tus marcadores aqui.', en: 'When the calendar is available, you will capture your scores here.' },
    kickoff:      { es: 'Inicio',                    en: 'Kickoff'                    },
    phase:        { es: 'Fase',                      en: 'Phase'                      },
    venue:        { es: 'Sede',                      en: 'Venue'                      },
    saved:        { es: 'Guardado',                  en: 'Saved'                      },
    unsaved:      { es: 'Sin guardar',               en: 'Unsaved'                    },
    locked:       { es: 'Bloqueado',                 en: 'Locked'                     },
    score:        { es: 'Marcador',                  en: 'Score'                      },
    home:         { es: 'Local',                     en: 'Home'                       },
    away:         { es: 'Visitante',                 en: 'Away'                       },
    submit:       { es: 'Guardar prediccion',        en: 'Save prediction'            },
    update:       { es: 'Actualizar',                en: 'Update'                     },
    success:      { es: 'Prediccion guardada',       en: 'Prediction saved'           },
    error:        { es: 'No se pudo guardar la prediccion', en: 'Could not save prediction' },
    exactHint:    { es: 'Los marcadores se cierran al iniciar el partido.', en: 'Scores lock when the match starts.' },
    points:       { es: 'pts',                       en: 'pts'                        },
  },
  ticker: {
    live:     { es: 'Tipo de cambio en tiempo real', en: 'Live exchange rate'      },
    outdated: { es: 'tipo de cambio desactualizado', en: 'exchange rate is stale'  },
  },
}

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: string) => string;
}

const I18nContext = createContext<I18nContextValue | null>(null);

function getTranslation(key: string, locale: Locale): string | undefined {
  let node: LeafEntry | TranslationNode | undefined = translations;
  for (const part of key.split(".")) {
    if (!node || typeof node === "string") return undefined;
    node = (node as TranslationNode)[part];
  }
  if (node && typeof node === "object" && locale in node) {
    return (node as LeafEntry)[locale];
  }
  return undefined;
}

export function I18nProvider({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const [locale, setLocale] = useState<Locale>("es");

  useEffect(() => {
    const stored = globalThis.localStorage.getItem("quiniela-locale");
    if (stored === "es" || stored === "en") {
      setLocale(stored);
      document.documentElement.lang = stored;
    }
  }, []);

  const value = useMemo<I18nContextValue>(
    () => ({
      locale,
      setLocale: (nextLocale) => {
        setLocale(nextLocale);
        globalThis.localStorage.setItem("quiniela-locale", nextLocale);
        document.documentElement.lang = nextLocale;
      },
      t: (key) =>
        getTranslation(key, locale) ?? getTranslation(key, "es") ?? key,
    }),
    [locale],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) throw new Error("useI18n must be used within I18nProvider");
  return context;
}

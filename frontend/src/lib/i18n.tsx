"use client";

import { createContext, useContext, useEffect, useMemo, useState } from "react";

export type Locale = "es" | "en";

type LeafEntry = Record<Locale, string>;
type TranslationNode = { [key: string]: LeafEntry | TranslationNode };

const localeTags: Record<Locale, string> = {
  es: "es-GT",
  en: "en-GB",
};

const teamTranslations: Record<string, LeafEntry> = {
  "algeria": { es: "Argelia", en: "Algeria" },
  "argentina": { es: "Argentina", en: "Argentina" },
  "australia": { es: "Australia", en: "Australia" },
  "belgium": { es: "Bélgica", en: "Belgium" },
  "brazil": { es: "Brasil", en: "Brazil" },
  "canada": { es: "Canadá", en: "Canada" },
  "cape verde": { es: "Cabo Verde", en: "Cape Verde" },
  "colombia": { es: "Colombia", en: "Colombia" },
  "croatia": { es: "Croacia", en: "Croatia" },
  "curacao": { es: "Curazao", en: "Curaçao" },
  "curaçao": { es: "Curazao", en: "Curaçao" },
  "czech republic": { es: "República Checa", en: "Czech Republic" },
  "ecuador": { es: "Ecuador", en: "Ecuador" },
  "egypt": { es: "Egipto", en: "Egypt" },
  "england": { es: "Inglaterra", en: "England" },
  "france": { es: "Francia", en: "France" },
  "germany": { es: "Alemania", en: "Germany" },
  "ghana": { es: "Ghana", en: "Ghana" },
  "guatemala": { es: "Guatemala", en: "Guatemala" },
  "haiti": { es: "Haití", en: "Haiti" },
  "italy": { es: "Italia", en: "Italy" },
  "ivory coast": { es: "Costa de Marfil", en: "Ivory Coast" },
  "cote d'ivoire": { es: "Costa de Marfil", en: "Côte d'Ivoire" },
  "côte d'ivoire": { es: "Costa de Marfil", en: "Côte d'Ivoire" },
  "japan": { es: "Japón", en: "Japan" },
  "jordan": { es: "Jordania", en: "Jordan" },
  "mexico": { es: "México", en: "Mexico" },
  "morocco": { es: "Marruecos", en: "Morocco" },
  "new zealand": { es: "Nueva Zelanda", en: "New Zealand" },
  "nigeria": { es: "Nigeria", en: "Nigeria" },
  "panama": { es: "Panamá", en: "Panama" },
  "paraguay": { es: "Paraguay", en: "Paraguay" },
  "portugal": { es: "Portugal", en: "Portugal" },
  "saudi arabia": { es: "Arabia Saudita", en: "Saudi Arabia" },
  "scotland": { es: "Escocia", en: "Scotland" },
  "south africa": { es: "Sudáfrica", en: "South Africa" },
  "south korea": { es: "Corea del Sur", en: "South Korea" },
  "spain": { es: "España", en: "Spain" },
  "switzerland": { es: "Suiza", en: "Switzerland" },
  "tunisia": { es: "Túnez", en: "Tunisia" },
  "uruguay": { es: "Uruguay", en: "Uruguay" },
  "usa": { es: "Estados Unidos", en: "United States" },
  "united states": { es: "Estados Unidos", en: "United States" },
  "uzbekistan": { es: "Uzbekistán", en: "Uzbekistan" },
};

const phaseTranslations: Record<string, LeafEntry> = {
  "group_stage": { es: "Fase de Grupos", en: "Group Stage" },
  "group stage": { es: "Fase de Grupos", en: "Group Stage" },
  "round_of_32": { es: "Dieciseisavos de Final", en: "Round of 32" },
  "round_of_16": { es: "Octavos de Final", en: "Round of 16" },
  "quarter_final": { es: "Cuartos de Final", en: "Quarter-Final" },
  "quarterfinal": { es: "Cuartos de Final", en: "Quarter-Final" },
  "semi_final": { es: "Semifinal", en: "Semi-Final" },
  "semifinal": { es: "Semifinal", en: "Semi-Final" },
  "third_place": { es: "Tercer Lugar", en: "Third Place" },
  "final": { es: "Final", en: "Final" },
};

// prettier-ignore
const translations: TranslationNode = {
  common: {
    brand:       { es: 'Quiniela 26',          en: 'Quiniela 26'          },
    event:       { es: 'Mundial 2026',         en: 'World Cup 2026'       },
    tournaments: { es: 'Fan Fest',             en: 'Fan Fest'             },
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
    secondary: { es: 'Ver Fan Fest',                en: 'View Fan Fest'                },
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
    exploreTournaments: { es: 'Explorar Fan Fest',             en: 'Explore Fan Fest'             },
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
    title:      { es: 'Fan Fest',                                              en: 'Fan Fest'                                 },
    subtitle:   { es: 'Explora quinielas disponibles, arma tu ruta por grupos y revisa el estado de tus picks.', en: 'Explore available pools, build your group-stage route, and review your picks.' },
    emptyTitle: { es: 'No hay torneos disponibles',                            en: 'No tournaments available'                 },
    emptyDesc:  { es: 'Vuelve pronto para ver las proximas quinielas',         en: 'Check back soon for upcoming pools'       },
    prize:      { es: 'Pozo',                                                  en: 'Prize pool'                               },
    entry:      { es: 'Inscripcion',                                           en: 'Entry'                                    },
    enter:      { es: 'Entrar',                                                en: 'Enter'                                    },
    details:    { es: 'Ver detalles',                                          en: 'View details'                             },
    members:    { es: 'miembros',                                              en: 'members'                                  },
    createPick: { es: 'Crear quiniela',                                         en: 'Create quiniela'                          },
    marketplace:{ es: 'Quinielas destacadas',                                  en: 'Featured pools'                           },
    availablePools:{ es: 'Pools disponibles',                                  en: 'Available pools'                          },
    options:    { es: 'opciones',                                               en: 'options'                                  },
  },
  predictions: {
    title:        { es: 'Panel de predicciones',     en: 'Prediction panel'          },
    subtitle:     { es: 'Lista de predicciones por partido. Filtra por grupo y guarda marcadores antes del inicio.', en: 'Match-by-match prediction list. Filter by group and save scores before kick-off.' },
    filterAll:    { es: 'Todos',                     en: 'All'                        },
    filterPending:{ es: 'Pendientes',                en: 'Pending'                    },
    filterSaved:  { es: 'Guardados',                 en: 'Saved'                      },
    groupSelector:{ es: 'Grupos',                    en: 'Groups'                     },
    groupSelectorAll:{ es: 'Mostrando partidos de todos los grupos', en: 'Showing matches from all groups' },
    groupSelected:{ es: 'Mostrando Grupo',           en: 'Showing Group'              },
    groupAll:     { es: 'Todos',                     en: 'All'                        },
    matches:      { es: 'partidos',                  en: 'matches'                    },
    noMatches:    { es: 'No hay partidos programados', en: 'No scheduled matches'     },
    noMatchesDesc:{ es: 'Cuando el calendario este disponible, podras capturar tus marcadores aqui.', en: 'When the calendar is available, you will capture your scores here.' },
    kickoff:      { es: 'Inicio',                    en: 'Kickoff'                    },
    timezone:     { es: 'Hora local',                en: 'Local time'                 },
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
  timeZone: string;
  setLocale: (locale: Locale) => void;
  t: (key: string) => string;
  formatKickoff: (iso: string | null | undefined) => string;
  phaseName: (phase: string | null | undefined) => string;
  teamName: (name: string | null | undefined) => string;
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
  const [timeZone, setTimeZone] = useState("UTC");

  useEffect(() => {
    const stored = globalThis.localStorage.getItem("quiniela-locale");
    if (stored === "es" || stored === "en") {
      setLocale(stored);
      document.documentElement.lang = stored;
    }
    setTimeZone(
      Intl.DateTimeFormat().resolvedOptions().timeZone ||
        "America/Guatemala",
    );
  }, []);

  const value = useMemo<I18nContextValue>(
    () => ({
      locale,
      timeZone,
      setLocale: (nextLocale) => {
        setLocale(nextLocale);
        globalThis.localStorage.setItem("quiniela-locale", nextLocale);
        document.documentElement.lang = nextLocale;
      },
      t: (key) =>
        getTranslation(key, locale) ?? getTranslation(key, "es") ?? key,
      formatKickoff: (iso) => formatLocalizedDateTime(iso, locale, timeZone),
      phaseName: (phase) => translateDictionaryValue(phase, phaseTranslations, locale),
      teamName: (name) => translateDictionaryValue(name, teamTranslations, locale),
    }),
    [locale, timeZone],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

function normalizeDictionaryKey(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ");
}

function translateDictionaryValue(
  value: string | null | undefined,
  dictionary: Record<string, LeafEntry>,
  locale: Locale,
): string {
  if (!value) return "—";
  const direct = dictionary[value.trim().toLowerCase()];
  const normalised = dictionary[normalizeDictionaryKey(value)];
  return (direct ?? normalised)?.[locale] ?? value;
}

function formatLocalizedDateTime(
  iso: string | null | undefined,
  locale: Locale,
  timeZone: string,
): string {
  if (!iso) return "—";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "—";

  return new Intl.DateTimeFormat(localeTags[locale], {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    timeZone,
    timeZoneName: "short",
  }).format(date);
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) throw new Error("useI18n must be used within I18nProvider");
  return context;
}

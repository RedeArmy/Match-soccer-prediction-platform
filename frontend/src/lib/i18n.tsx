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
  // Group A
  "mexico":                 { es: "México",                 en: "Mexico"                 },
  "south africa":           { es: "Sudáfrica",              en: "South Africa"           },
  "south korea":            { es: "Corea del Sur",          en: "South Korea"            },
  "czechia":                { es: "República Checa",        en: "Czechia"                },
  "czech republic":         { es: "República Checa",        en: "Czech Republic"         },
  // Group B
  "canada":                 { es: "Canadá",                 en: "Canada"                 },
  "bosnia and herzegovina": { es: "Bosnia y Herzegovina",   en: "Bosnia and Herzegovina" },
  "qatar":                  { es: "Catar",                  en: "Qatar"                  },
  "switzerland":            { es: "Suiza",                  en: "Switzerland"            },
  // Group C
  "brazil":                 { es: "Brasil",                 en: "Brazil"                 },
  "morocco":                { es: "Marruecos",              en: "Morocco"                },
  "haiti":                  { es: "Haití",                  en: "Haiti"                  },
  "scotland":               { es: "Escocia",                en: "Scotland"               },
  // Group D
  "united states":          { es: "Estados Unidos",         en: "United States"          },
  "usa":                    { es: "Estados Unidos",         en: "United States"          },
  "paraguay":               { es: "Paraguay",               en: "Paraguay"               },
  "australia":              { es: "Australia",              en: "Australia"              },
  "türkiye":                { es: "Turquía",                en: "Türkiye"                },
  "turkiye":                { es: "Turquía",                en: "Türkiye"                },
  // Group E
  "germany":                { es: "Alemania",               en: "Germany"                },
  "curaçao":                { es: "Curazao",                en: "Curaçao"                },
  "curacao":                { es: "Curazao",                en: "Curaçao"                },
  "ivory coast":            { es: "Costa de Marfil",        en: "Ivory Coast"            },
  "cote d'ivoire":          { es: "Costa de Marfil",        en: "Côte d'Ivoire"          },
  "côte d'ivoire":          { es: "Costa de Marfil",        en: "Côte d'Ivoire"          },
  "ecuador":                { es: "Ecuador",                en: "Ecuador"                },
  // Group F
  "netherlands":            { es: "Países Bajos",           en: "Netherlands"            },
  "japan":                  { es: "Japón",                  en: "Japan"                  },
  "sweden":                 { es: "Suecia",                 en: "Sweden"                 },
  "tunisia":                { es: "Túnez",                  en: "Tunisia"                },
  // Group G
  "belgium":                { es: "Bélgica",                en: "Belgium"                },
  "egypt":                  { es: "Egipto",                 en: "Egypt"                  },
  "iran":                   { es: "Irán",                   en: "Iran"                   },
  "new zealand":            { es: "Nueva Zelanda",          en: "New Zealand"            },
  // Group H
  "spain":                  { es: "España",                 en: "Spain"                  },
  "cape verde":             { es: "Cabo Verde",             en: "Cape Verde"             },
  "saudi arabia":           { es: "Arabia Saudita",         en: "Saudi Arabia"           },
  "uruguay":                { es: "Uruguay",                en: "Uruguay"                },
  // Group I
  "france":                 { es: "Francia",                en: "France"                 },
  "senegal":                { es: "Senegal",                en: "Senegal"                },
  "iraq":                   { es: "Irak",                   en: "Iraq"                   },
  "norway":                 { es: "Noruega",                en: "Norway"                 },
  // Group J
  "argentina":              { es: "Argentina",              en: "Argentina"              },
  "algeria":                { es: "Argelia",                en: "Algeria"                },
  "austria":                { es: "Austria",                en: "Austria"                },
  "jordan":                 { es: "Jordania",               en: "Jordan"                 },
  // Group K
  "portugal":               { es: "Portugal",               en: "Portugal"               },
  "dr congo":               { es: "Rep. Dem. del Congo",    en: "DR Congo"               },
  "uzbekistan":             { es: "Uzbekistán",             en: "Uzbekistan"             },
  "colombia":               { es: "Colombia",               en: "Colombia"               },
  // Group L
  "england":                { es: "Inglaterra",             en: "England"                },
  "croatia":                { es: "Croacia",                en: "Croatia"                },
  "ghana":                  { es: "Ghana",                  en: "Ghana"                  },
  "panama":                 { es: "Panamá",                 en: "Panama"                 },
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
    brand:       { es: 'Kiniela 26',          en: 'Kiniela 26'          },
    event:       { es: 'Mundial 2026',         en: 'World Cup 2026'       },
    tournaments: { es: 'Fan Fest',             en: 'Fan Fest'             },
    kinielas:   { es: 'Kinielas',            en: 'Kinielas'            },
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
    back:        { es: 'Volver',                  en: 'Back'                 },
    cancel:      { es: 'Cancelar',               en: 'Cancel'               },
    you:         { es: 'tú',                      en: 'you'                  },
    notFound:    { es: 'Grupo no encontrado.',    en: 'Group not found.'     },
  },
  group: {
    leaderboard:         { es: 'Clasificación',              en: 'Leaderboard'              },
    members:             { es: 'Participantes',              en: 'Participants'             },
    membersActive:       { es: 'Miembros activos',           en: 'Active members'           },
    membersPending:      { es: 'Pendientes de aprobación',   en: 'Pending approval'         },
    noScores:            { es: 'Aún no hay puntuaciones.',   en: 'No scores yet.'           },
    noMembers:           { es: 'Sin participantes aún.',     en: 'No participants yet.'     },
    leave:               { es: 'Salir del grupo',            en: 'Leave group'              },
    leaveConfirm:        { es: '¿Seguro que quieres salir?', en: 'Are you sure you want to leave?' },
    leaveConfirmDetail:  { es: 'Podrás volver a unirte con el código de invitación, pero necesitarás ser aprobado nuevamente.', en: 'You can rejoin with the invite code, but will need to be approved again.' },
    leaveConfirmBtn:     { es: 'Sí, salir',                 en: 'Yes, leave'               },
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
    eyebrow:   { es: 'Kiniela internacional',     en: 'International football pool'  },
    title:     { es: 'Kiniela Mundial 2026',       en: 'World Cup 2026 Kiniela'      },
    subtitle:  { es: 'Predice marcadores, administra tus kinielas y compite en una experiencia moderna inspirada en Canada, Mexico y Estados Unidos.', en: 'Predict scores, manage your pools, and compete in a modern experience inspired by Canada, Mexico, and the United States.' },
    primary:   { es: 'Entrar al tablero',           en: 'Open dashboard'               },
    secondary: { es: 'Ver Fan Fest',                en: 'View Fan Fest'                },
    rate:      { es: 'Tipo de cambio',              en: 'Exchange rate'                },
    buy:       { es: 'compra',                      en: 'buy'                          },
    howTitle:  { es: 'Flujo de juego',              en: 'Game flow'                    },
    step1Title:{ es: 'Unete',                       en: 'Join'                         },
    step1Desc: { es: 'Crea tu cuenta, verifica tu perfil y entra a una kiniela.',     en: 'Create your account, verify your profile, and enter a pool.'      },
    step2Title:{ es: 'Predice',                     en: 'Predict'                      },
    step2Desc: { es: 'Registra marcadores antes del silbatazo inicial de cada partido.', en: 'Submit scores before kickoff for every match.'                  },
    step3Title:{ es: 'Compite',                     en: 'Compete'                      },
    step3Desc: { es: 'Suma puntos, consulta rankings y cobra premios en GTQ.',         en: 'Earn points, check rankings, and collect prizes in GTQ.'          },
    opsTitle:  { es: 'Operaciones listas para crecer', en: 'Built for scale'           },
    opsCopy:   { es: 'Pagos, KYC, notificaciones, rankings y observabilidad viven en un producto compacto y preparado para torneos internacionales.', en: 'Payments, KYC, notifications, rankings, and observability live in a compact product ready for international tournaments.' },
  },
  groups: {
    eyebrow:          { es: 'Kinielas',                        en: 'Pools'                          },
    createTitle:      { es: 'Crear kiniela',                   en: 'Create pool'                    },
    joinTitle:        { es: 'Unirse a kiniela',                en: 'Join pool'                      },
    tabCreate:        { es: 'Crear',                            en: 'Create'                         },
    tabJoin:          { es: 'Unirse',                           en: 'Join'                           },
    nameLabel:        { es: 'Nombre del grupo',                 en: 'Group name'                     },
    namePlaceholder:  { es: 'Ej. Amigos del trabajo',           en: 'E.g. Work friends'              },
    entryFeeLabel:    { es: 'Cuota de inscripción',              en: 'Entry fee'                      },
    entryFeeHint:     { es: 'Deja en 0 para kiniela gratuita', en: 'Set to 0 for a free pool'      },
    inviteCodeLabel:  { es: 'Código de invitación',             en: 'Invite code'                    },
    inviteCodeHint:   { es: 'Comparte este código para que otros se unan a tu kiniela.', en: 'Share this code so others can join your pool.' },
    createAction:     { es: 'Crear kiniela',                   en: 'Create pool'                    },
    joinAction:       { es: 'Unirse',                           en: 'Join'                           },
    createSuccess:    { es: '¡Kiniela creada!',                en: 'Pool created!'                  },
    createError:      { es: 'No se pudo crear la kiniela.',    en: 'Could not create the pool.'     },
    joinError:        { es: 'Código inválido o grupo no disponible.', en: 'Invalid code or group unavailable.' },
    done:             { es: 'Listo',                            en: 'Done'                           },
    createBtn:        { es: 'Crear',                            en: 'Create'                         },
    joinBtn:          { es: 'Unirse',                           en: 'Join'                           },
    pendingTitle:     { es: 'Solicitudes pendientes',           en: 'Pending requests'               },
    noPending:        { es: 'No hay solicitudes pendientes.',   en: 'No pending requests.'           },
    approve:          { es: 'Aceptar',                          en: 'Accept'                         },
    reject:           { es: 'Rechazar',                         en: 'Reject'                         },
    pendingBadge:     { es: 'Pendiente',                        en: 'Pending'                        },
  },
  dashboard: {
    hello:              { es: 'Hola',                          en: 'Hello'                        },
    player:             { es: 'Jugador',                       en: 'Player'                       },
    subtitle:           { es: 'Centro de control personal',    en: 'Personal command center'      },
    kycTitle:           { es: 'Verifica tu identidad',         en: 'Verify your identity'         },
    kycCopy:            { es: 'Completa tu KYC para poder hacer retiros.', en: 'Complete KYC to unlock withdrawals.' },
    kycAction:          { es: 'Verificar ahora',               en: 'Verify now'                   },
    myPools:            { es: 'Mis kinielas',                 en: 'My pools'                     },
    exploreTournaments: { es: 'Kinielas',                      en: 'Kinielas'                    },
    noPools:            { es: 'Aun no tienes kinielas',       en: 'No pools yet'                 },
    noPoolsDesc:        { es: 'Unete a un torneo para empezar a predecir', en: 'Join a tournament to start predicting' },
    recentTransactions: { es: 'Ultimas transacciones',         en: 'Recent transactions'          },
    noTransactions:     { es: 'Sin transacciones aun',         en: 'No transactions yet'          },
    participants:       { es: 'participantes',                 en: 'participants'                 },
    commandTitle:       { es: 'Vista de torneo',               en: 'Tournament view'              },
    commandCopy:        { es: 'Gestiona balance, kinielas y predicciones desde un panel compacto.', en: 'Manage balance, pools, and predictions from a compact workspace.' },
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
    subtitle:   { es: 'Explora kinielas disponibles, arma tu ruta por grupos y revisa el estado de tus picks.', en: 'Explore available pools, build your group-stage route, and review your picks.' },
    emptyTitle: { es: 'No hay torneos disponibles',                            en: 'No tournaments available'                 },
    emptyDesc:  { es: 'Vuelve pronto para ver las proximas kinielas',         en: 'Check back soon for upcoming pools'       },
    prize:      { es: 'Pozo',                                                  en: 'Prize pool'                               },
    entry:      { es: 'Inscripcion',                                           en: 'Entry'                                    },
    enter:      { es: 'Entrar',                                                en: 'Enter'                                    },
    details:    { es: 'Ver detalles',                                          en: 'View details'                             },
    members:    { es: 'miembros',                                              en: 'members'                                  },
    createPick: { es: 'Crear kiniela',                                         en: 'Create kiniela'                          },
    marketplace:{ es: 'Kinielas destacadas',                                  en: 'Featured pools'                           },
    availablePools:{ es: 'Pools disponibles',                                  en: 'Available pools'                          },
    options:    { es: 'opciones',                                               en: 'options'                                  },
  },
  predictions: {
    title:        { es: 'Panel de predicciones',     en: 'Prediction panel'          },
    subtitle:     { es: 'Lista de predicciones por partido. Filtra por grupo y guarda marcadores antes del inicio.', en: 'Match-by-match prediction list. Filter by group and save scores before kick-off.' },
    filterAll:    { es: 'Todos',                     en: 'All'                        },
    filterPending:{ es: 'Pendientes',                en: 'Pending'                    },
    filterSaved:  { es: 'Guardados',                 en: 'Saved'                      },
    viewByGroup:  { es: 'Grupos',                    en: 'Groups'                     },
    viewByDay:    { es: 'Hoy',                       en: 'Today'                      },
    groupSelector:{ es: 'Grupos',                    en: 'Groups'                     },
    groupSelectorAll:{ es: 'Mostrando partidos de todos los grupos', en: 'Showing matches from all groups' },
    groupSelected:{ es: 'Mostrando Grupo',           en: 'Showing Group'              },
    groupAll:     { es: 'Todos',                     en: 'All'                        },
    matches:      { es: 'partidos',                  en: 'matches'                    },
    noMatches:    { es: 'No hay partidos programados', en: 'No scheduled matches'     },
    noMatchesDesc:{ es: 'Cuando el calendario este disponible, podras capturar tus marcadores aqui.', en: 'When the calendar is available, you will capture your scores here.' },
    loadError:    { es: 'No se pudieron cargar los partidos', en: 'Could not load matches' },
    loadErrorDesc:{ es: 'Intenta recargar la pagina. Si el problema persiste, contacta al administrador.', en: 'Try reloading the page. If the problem persists, contact the administrator.' },
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
    hourCycle: "h23",
    timeZone,
  }).format(date);
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) throw new Error("useI18n must be used within I18nProvider");
  return context;
}

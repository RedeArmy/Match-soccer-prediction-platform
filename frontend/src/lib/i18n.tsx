"use client";

import { createContext, useContext, useEffect, useMemo, useState } from "react";

export type Locale = "es" | "en";

type LeafEntry = Record<Locale, string>;
type TranslationNode = { [key: string]: LeafEntry | TranslationNode };

const localeTags: Record<Locale, string> = {
  es: "es-GT",
  en: "en-GB",
};

const accountTypeTranslations: Record<string, LeafEntry> = {
  "ahorros gtq": { es: "Ahorros GTQ", en: "Savings GTQ" },
  "ahorros usd": { es: "Ahorros USD", en: "Savings USD" },
  "monetaria gtq": { es: "Monetaria GTQ", en: "Checking GTQ" },
  "monetaria usd": { es: "Monetaria USD", en: "Checking USD" },
  "tarjeta de credito": { es: "Tarjeta de Crédito", en: "Credit Card" },
};

const teamTranslations: Record<string, LeafEntry> = {
  // Group A
  mexico: { es: "México", en: "Mexico" },
  "south africa": { es: "Sudáfrica", en: "South Africa" },
  "south korea": { es: "Corea del Sur", en: "South Korea" },
  czechia: { es: "República Checa", en: "Czechia" },
  "czech republic": { es: "República Checa", en: "Czech Republic" },
  // Group B
  canada: { es: "Canadá", en: "Canada" },
  "bosnia and herzegovina": {
    es: "Bosnia y Herzegovina",
    en: "Bosnia and Herzegovina",
  },
  qatar: { es: "Catar", en: "Qatar" },
  switzerland: { es: "Suiza", en: "Switzerland" },
  // Group C
  brazil: { es: "Brasil", en: "Brazil" },
  morocco: { es: "Marruecos", en: "Morocco" },
  haiti: { es: "Haití", en: "Haiti" },
  scotland: { es: "Escocia", en: "Scotland" },
  // Group D
  "united states": { es: "Estados Unidos", en: "United States" },
  usa: { es: "Estados Unidos", en: "United States" },
  paraguay: { es: "Paraguay", en: "Paraguay" },
  australia: { es: "Australia", en: "Australia" },
  türkiye: { es: "Turquía", en: "Türkiye" },
  turkiye: { es: "Turquía", en: "Türkiye" },
  // Group E
  germany: { es: "Alemania", en: "Germany" },
  curaçao: { es: "Curazao", en: "Curaçao" },
  curacao: { es: "Curazao", en: "Curaçao" },
  "ivory coast": { es: "Costa de Marfil", en: "Ivory Coast" },
  "cote d'ivoire": { es: "Costa de Marfil", en: "Côte d'Ivoire" },
  "côte d'ivoire": { es: "Costa de Marfil", en: "Côte d'Ivoire" },
  ecuador: { es: "Ecuador", en: "Ecuador" },
  // Group F
  netherlands: { es: "Países Bajos", en: "Netherlands" },
  japan: { es: "Japón", en: "Japan" },
  sweden: { es: "Suecia", en: "Sweden" },
  tunisia: { es: "Túnez", en: "Tunisia" },
  // Group G
  belgium: { es: "Bélgica", en: "Belgium" },
  egypt: { es: "Egipto", en: "Egypt" },
  iran: { es: "Irán", en: "Iran" },
  "new zealand": { es: "Nueva Zelanda", en: "New Zealand" },
  // Group H
  spain: { es: "España", en: "Spain" },
  "cape verde": { es: "Cabo Verde", en: "Cape Verde" },
  "saudi arabia": { es: "Arabia Saudita", en: "Saudi Arabia" },
  uruguay: { es: "Uruguay", en: "Uruguay" },
  // Group I
  france: { es: "Francia", en: "France" },
  senegal: { es: "Senegal", en: "Senegal" },
  iraq: { es: "Irak", en: "Iraq" },
  norway: { es: "Noruega", en: "Norway" },
  // Group J
  argentina: { es: "Argentina", en: "Argentina" },
  algeria: { es: "Argelia", en: "Algeria" },
  austria: { es: "Austria", en: "Austria" },
  jordan: { es: "Jordania", en: "Jordan" },
  // Group K
  portugal: { es: "Portugal", en: "Portugal" },
  "dr congo": { es: "Rep. Dem. del Congo", en: "DR Congo" },
  uzbekistan: { es: "Uzbekistán", en: "Uzbekistan" },
  colombia: { es: "Colombia", en: "Colombia" },
  // Group L
  england: { es: "Inglaterra", en: "England" },
  croatia: { es: "Croacia", en: "Croatia" },
  ghana: { es: "Ghana", en: "Ghana" },
  panama: { es: "Panamá", en: "Panama" },
};

const phaseTranslations: Record<string, LeafEntry> = {
  group_stage: { es: "Fase de Grupos", en: "Group Stage" },
  "group stage": { es: "Fase de Grupos", en: "Group Stage" },
  round_of_16: { es: "Octavos de Final", en: "Round of 16" },
  quarter_final: { es: "Cuartos de Final", en: "Quarter-Final" },
  quarterfinal: { es: "Cuartos de Final", en: "Quarter-Final" },
  semi_final: { es: "Semifinal", en: "Semi-Final" },
  semifinal: { es: "Semifinal", en: "Semi-Final" },
  third_place: { es: "Tercer Lugar", en: "Third Place" },
  final: { es: "Final", en: "Final" },
};

// prettier-ignore
const translations: TranslationNode = {
  common: {
    brand:       { es: 'Kiniela 26',          en: 'Kiniela 26'          },
    event:       { es: 'Copa 2026',             en: 'Cup 2026'             },
    tournaments: { es: 'Fan Fest',             en: 'Fan Fest'             },
    kinielas:   { es: 'Kinielas',            en: 'Kinielas'            },
    goHome:      { es: 'Página Principal',    en: 'Home'                 },
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
    createdBy:   { es: 'Creado por',                                             en: 'Created by'                        },
    back:        { es: 'Volver',                  en: 'Back'                 },
    cancel:      { es: 'Cancelar',               en: 'Cancel'               },
    you:         { es: 'tú',                      en: 'you'                  },
    notFound:    { es: 'Grupo no encontrado.',    en: 'Group not found.'     },
    live:        { es: 'EN VIVO',                 en: 'LIVE'                 },
    scrollLeft:  { es: 'Desplazar a la izquierda', en: 'Scroll left'         },
    scrollRight: { es: 'Desplazar a la derecha',   en: 'Scroll right'        },
    prev:        { es: 'Anterior',                  en: 'Previous'            },
    next:        { es: 'Siguiente',                 en: 'Next'                },
  },
  group: {
    leaderboard:         { es: 'Clasificación',              en: 'Leaderboard'              },
    members:             { es: 'Participantes',              en: 'Participants'             },
    membersActive:       { es: 'Miembros activos',           en: 'Active members'           },
    membersPending:      { es: 'Pendientes de aprobación',   en: 'Pending approval'         },
    noScores:            { es: 'Aún no hay puntuaciones.',   en: 'No scores yet.'           },
    noMembers:           { es: 'Sin participantes aún.',     en: 'No participants yet.'     },
    leave:               { es: 'Salir del grupo',            en: 'Leave group'              },
    settings:            { es: 'Configuración',              en: 'Settings'                 },
    tournamentMode:      { es: 'Modo de torneo',             en: 'Tournament mode'          },
    tournamentModeFree:  { es: 'Gratis — sin premios en efectivo', en: 'Free — no cash prizes' },
    upgradeToPremium:    { es: 'Activar Premium',            en: 'Activate Premium'         },
    ownerLabel:          { es: 'Líder',                      en: 'Leader'                   },
    creatorLabel:        { es: 'Creador',                    en: 'Creator'                  },
    modeGeneral:         { es: 'General (cuota única)',       en: 'General (single entry fee)' },
    modeGeneralDesc:     { es: 'Premio al final del torneo para los mejores clasificados.', en: 'Prize at the end of the tournament for top finishers.' },
    modeRound:           { es: 'Por jornada',                en: 'Per matchday'             },
    modeRoundDesc:       { es: 'Premio a 1.° y 2.° por cada jornada jugada.', en: 'Prize to 1st and 2nd for each played matchday.' },
    prizePoolGeneral:    { es: 'Premio general',             en: 'General prize pool'       },
    prizePoolRound:      { es: 'Premio por jornada',         en: 'Per-matchday prize'       },
    roundLabel:          { es: 'Jornada',                    en: 'Matchday'                 },
    round1:              { es: 'J1',                          en: 'MD1'                      },
    round2:              { es: 'J2',                          en: 'MD2'                      },
    round3:              { es: 'J3',                          en: 'MD3'                      },
    round4:              { es: '16avos',                      en: 'R16'                      },
    round6:              { es: 'Cuartos',                     en: 'QF'                       },
    round7:              { es: 'Semis',                       en: 'SF'                       },
    round8:              { es: '3.°',                         en: '3rd'                      },
    round9:              { es: 'Final',                       en: 'Final'                    },
    premiumReady:        { es: 'El grupo tiene suficientes miembros para activar Premium. Selecciona los modos abajo.', en: 'The group has enough members to activate Premium. Select modes below.' },
    premiumNotReady:     { es: 'Se necesitan al menos {min} miembros para activar Premium.', en: 'At least {min} members are required to activate Premium.' },
    leaveConfirm:        { es: '¿Seguro que quieres salir?', en: 'Are you sure you want to leave?' },
    leaveConfirmDetail:  { es: 'Podrás volver a unirte con el código de invitación, pero necesitarás ser aprobado nuevamente.', en: 'You can rejoin with the invite code, but will need to be approved again.' },
    leaveConfirmBtn:     { es: 'Sí, salir',                 en: 'Yes, leave'               },
    settingsError:           { es: 'No se pudo guardar la configuración.', en: 'Could not save settings.' },
    settingsErrorMinMembers: { es: 'Se necesitan al menos {min} miembros activos para activar Premium.', en: 'At least {min} active members are required to activate Premium.' },
    settingsErrorBalance:    { es: 'Uno o más miembros no tienen saldo suficiente para la cuota.', en: 'One or more members have insufficient balance for the entry fee.' },
    // Ajustes tab labels
    tabSettings:             { es: 'Ajustes',                             en: 'Settings'                   },
    // Require-approval toggle
    requireApprovalLabel:    { es: 'Requerir aprobación',                 en: 'Require approval'            },
    requireApprovalOnDesc:   { es: 'Los nuevos miembros deben ser aprobados por un miembro activo antes de unirse.', en: 'New members must be approved by an active member before joining.' },
    requireApprovalOffDesc:  { es: 'Los usuarios se unen automáticamente al presentar el código de invitación, sin esperar aprobación.', en: 'Users join automatically when they present the invite code, without waiting for approval.' },
    // Score-from-zero feature
    scoreFromZeroLabel:          { es: 'Iniciar con 0 puntos',            en: 'Start from zero points'      },
    scoreFromZeroDesc:           { es: 'Al activar, los puntos de todos los miembros se reiniciarán a 0 en este grupo. Esta acción no puede revertirse.', en: "When activated, all members' points in this group will be reset to 0. This action cannot be reversed." },
    scoreFromZeroLockedDesc:     { es: 'Activado · Irreversible. Los nuevos miembros ingresan con 0 puntos en este grupo.', en: 'Activated · Irreversible. New members join with 0 points in this group.' },
    scoreFromZeroConfirmTitle:   { es: '¿Confirmar activación?',          en: 'Confirm activation?'         },
    scoreFromZeroConfirmItem1:   { es: 'Los puntos de todos los miembros actuales del grupo se reiniciarán a 0.', en: "All current members' points in this group will be reset to 0." },
    scoreFromZeroConfirmItem2:   { es: 'Los nuevos miembros que se unan también comenzarán con 0 puntos en este grupo.', en: 'New members who join will also start with 0 points in this group.' },
    scoreFromZeroConfirmItem3:   { es: 'Esta acción no puede revertirse.', en: 'This action cannot be reversed.' },
    scoreFromZeroConfirmItem4:   { es: 'Los puntos en otros grupos no se verán afectados.', en: 'Points in other groups will not be affected.' },
    scoreFromZeroConfirmBtn:     { es: 'Confirmar',                       en: 'Confirm'                     },
    scoreFromZeroActivateError:  { es: 'No se pudo activar. Intenta nuevamente.', en: 'Could not activate. Please try again.' },
  },
  status: {
    active:       { es: 'Activo',         en: 'Active'       },
    upcoming:     { es: 'Proximo',        en: 'Upcoming'     },
    ended:        { es: 'Finalizado',     en: 'Ended'        },
    pending:      { es: 'Pendiente',      en: 'Pending'      },
    approved:     { es: 'Aprobado',       en: 'Approved'     },
    rejected:     { es: 'Rechazado',      en: 'Rejected'     },
    captured:     { es: 'Acreditado',     en: 'Credited'     },
    expired:      { es: 'Vencido',        en: 'Expired'      },
    under_review: { es: 'En revisión',    en: 'Under review' },
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
    free:         { es: 'Gratis',          en: 'Free'         },
    premium:      { es: 'Premium',         en: 'Premium'      },
  },
  nav: {
    home:    { es: 'Inicio',   en: 'Home'    },
    matches: { es: 'Partidos', en: 'Matches' },
  },
  landing: {
    // Hero
    eyebrow:         { es: 'Kiniela Internacional · Torneo Internacional de Fútbol 2026',                                                                                    en: 'International Kiniela · International Football Tournament 2026'                                                                                  },
    heroLine1:       { es: 'Kiniela',                                                                                                                                        en: 'Football'                                                                                                                                        },
    heroGold:        { es: 'Mundial',                                                                                                                                        en: 'Kiniela'                                                                                                                                         },
    heroLine3:       { es: '2026',                                                                                                                                           en: '2026'                                                                                                                                            },
    subtitle:        { es: 'Predice marcadores, compite con amigos y gana premios reales en quetzales durante el torneo más grande del fútbol.',                             en: 'Predict scores, compete with friends and win real prizes in quetzals during the biggest football tournament.'                                    },
    primary:         { es: 'Empezar gratis',                                                                                                                                 en: 'Get started free'                                                                                                                                },
    secondary:       { es: 'Ver quinielas',                                                                                                                                  en: 'View pools'                                                                                                                                      },
    proof1:          { es: 'Sin costo para jugar',                                                                                                                           en: 'Free to play'                                                                                                                                    },
    proof2:          { es: 'Gana premios reales',                                                                                                                            en: 'Win real prizes'                                                                                                                                 },
    proof3:          { es: 'Resultados en vivo',                                                                                                                             en: 'Live results'                                                                                                                                    },
    // Preview panel
    previewLeader:   { es: 'Tabla de posiciones',                                                                                                                            en: 'Standings'                                                                                                                                       },
    previewYou:      { es: 'Tú',                                                                                                                                             en: 'You'                                                                                                                                             },
    previewPts:      { es: 'pts',                                                                                                                                            en: 'pts'                                                                                                                                             },
    previewLabel:    { es: 'Tu predicción · Grupo A',                                                                                                                        en: 'Your prediction · Group A'                                                                                                                       },
    previewScore:    { es: 'Marcador',                                                                                                                                       en: 'Score'                                                                                                                                           },
    previewSave:     { es: 'Guardar predicción',                                                                                                                             en: 'Save prediction'                                                                                                                                 },
    previewBadge:    { es: '+3 pts posibles',                                                                                                                                en: '+3 possible pts'                                                                                                                                 },
    // Stats strip
    statTeams:       { es: 'Selecciones',                                                                                                                                    en: 'Teams'                                                                                                                                           },
    statMatches:     { es: 'Partidos',                                                                                                                                       en: 'Matches'                                                                                                                                         },
    statCountries:   { es: 'Países sede',                                                                                                                                    en: 'Host countries'                                                                                                                                  },
    statMonth:       { es: 'Jun',                                                                                                                                            en: 'Jun'                                                                                                                                             },
    statYear:        { es: '2026',                                                                                                                                           en: '2026'                                                                                                                                            },
    // Scoring
    scoringEyebrow:  { es: 'Sistema de puntuación',                                                                                                                          en: 'Scoring system'                                                                                                                                  },
    scoringTitle:    { es: '¿Cómo se ganan puntos?',                                                                                                                         en: 'How are points earned?'                                                                                                                          },
    scoringSubtitle: { es: 'Cada partido vale. Cuanto más exacta sea tu predicción y más avanzado esté el torneo, más puntos sumas.',                                         en: 'Every match counts. The more accurate your prediction — and the further the tournament advances — the more points you earn.'                  },
    scoreExactLabel: { es: 'Marcador exacto',                                                                                                                                en: 'Exact score'                                                                                                                                     },
    scoreExactEx:    { es: 'Predijiste 2–1 y quedó 2–1',                                                                                                                    en: 'You predicted 2–1 and it finished 2–1'                                                                                                           },
    scoreResultLabel:{ es: 'Resultado correcto',                                                                                                                             en: 'Correct result'                                                                                                                                  },
    scoreResultEx:   { es: 'Predijiste 3–0 y quedó 2–1 (local gana)',                                                                                                       en: 'You predicted 3–0 and it finished 2–1 (home wins)'                                                                                               },
    scoreMissLabel:  { es: 'Sin acierto',                                                                                                                                    en: 'No match'                                                                                                                                        },
    scoreMissEx:     { es: 'Predijiste 0–0 y quedó 2–1',                                                                                                                    en: 'You predicted 0–0 and it finished 2–1'                                                                                                           },
    scoringTablePhase:{ es: 'Fase',                                                                                                                                          en: 'Phase'                                                                                                                                           },
    bonusLabel:      { es: 'Bonos extra:',                                                                                                                                   en: 'Extra bonuses:'                                                                                                                                  },
    bonusCopy:       { es: 'Si fallas el ganador pero aciertas la diferencia de goles, igual ganas puntos (+1 en fase de grupos, hasta +3 en la final). En fases eliminatorias también suman +1 pt si aciertas que el partido se decidió en tiempo extra, o +2 pts si fue en penales.', en: "If you miss the winner but get the goal margin right, you still score points (+1 in the group stage, up to +3 in the final). In knockout phases you also earn +1 pt for correctly calling an extra-time win, or +2 pts for a penalty-shootout win." },
    // Tournament format
    formatEyebrow:       { es: 'Formato del torneo 2026',                                                                                                                    en: 'Tournament 2026 Format'                                                                                                                          },
    formatTitle:         { es: 'Estructura del torneo',                                                                                                                      en: 'Tournament structure'                                                                                                                            },
    groupsTitle:         { es: 'Fase de Grupos',                                                                                                                             en: 'Group Stage'                                                                                                                                     },
    groupsDesc:          { es: '12 grupos de 4 selecciones cada uno. Cada equipo juega 3 partidos. Los 2 mejores de cada grupo avanzan, más los 8 mejores terceros.',        en: '12 groups of 4 teams each. Every team plays 3 matches. The top 2 from each group advance, plus the best 8 third-placed sides.'                 },
    groupsGroupsLabel:   { es: 'Grupos',                                                                                                                                     en: 'Groups'                                                                                                                                          },
    groupsMatchesLabel:  { es: 'Partidos',                                                                                                                                   en: 'Matches'                                                                                                                                         },
    groupsAdvanceLabel:  { es: 'Clasifican',                                                                                                                                 en: 'Advance'                                                                                                                                         },
    knockoutTitle:       { es: 'Fase Eliminatoria',                                                                                                                          en: 'Knockout Stage'                                                                                                                                  },
    knockoutDesc:        { es: 'Eliminación directa desde Ronda de 32 hasta la Gran Final. Cada partido decide quién continúa. Sin errores.',                               en: 'Single elimination from the Round of 32 to the Grand Final. Every match is decisive. No second chances.'                                       },
    ko32:                { es: 'Ronda de 32',                                                                                                                                en: 'Round of 32'                                                                                                                                     },
    ko16:                { es: 'Ronda de 16',                                                                                                                                en: 'Round of 16'                                                                                                                                     },
    koQF:                { es: 'Cuartos',                                                                                                                                    en: 'Quarter-finals'                                                                                                                                  },
    koSF:                { es: 'Semifinales',                                                                                                                                en: 'Semi-finals'                                                                                                                                     },
    koF:                 { es: 'Final',                                                                                                                                      en: 'Final'                                                                                                                                           },
    matchSingular:       { es: 'partido',                                                                                                                                    en: 'match'                                                                                                                                           },
    matchPlural:         { es: 'partidos',                                                                                                                                   en: 'matches'                                                                                                                                         },
    // Quiniela modes
    modesEyebrow:    { es: 'Tipos de quiniela',                                                                                                                              en: 'Pool types'                                                                                                                                      },
    modesTitle:      { es: 'Elige cómo competir',                                                                                                                            en: 'Choose how to compete'                                                                                                                           },
    freeModeName:    { es: 'Libre',                                                                                                                                          en: 'Free'                                                                                                                                            },
    freeModeTag:     { es: 'Gratis',                                                                                                                                         en: 'Free'                                                                                                                                            },
    freeModeDesc:    { es: 'Compite por puntos sin ningún costo. Ideal para seguir el torneo con amigos.',                                                                   en: 'Compete for points at no cost. Perfect for following the tournament with friends.'                                                                },
    freeF1:          { es: 'Sin costo de entrada',                                                                                                                           en: 'No entry fee'                                                                                                                                    },
    freeF2:          { es: 'Predicciones ilimitadas',                                                                                                                        en: 'Unlimited predictions'                                                                                                                           },
    freeF3:          { es: 'Tabla de posiciones en vivo',                                                                                                                    en: 'Live standings'                                                                                                                                  },
    freeF4:          { es: 'Sin verificación de identidad',                                                                                                                  en: 'No identity verification'                                                                                                                        },
    freeCta:         { es: 'Empezar gratis',                                                                                                                                 en: 'Get started free'                                                                                                                                },
    premModeName:    { es: 'Premium',                                                                                                                                        en: 'Premium'                                                                                                                                         },
    premModeTag:     { es: 'Premios en efectivo',                                                                                                                            en: 'Cash prizes'                                                                                                                                     },
    premModeDesc:    { es: 'Paga tu cuota, entra al pozo acumulado y retira tus ganancias en quetzales.',                                                                   en: 'Pay your entry fee, join the prize pool, and withdraw your winnings in quetzals.'                                                               },
    premF1:          { es: 'Cuota de entrada configurable',                                                                                                                  en: 'Configurable entry fee'                                                                                                                          },
    premF2:          { es: 'Pozo de premios acumulado',                                                                                                                      en: 'Accumulated prize pool'                                                                                                                          },
    premF3:          { es: 'Verificación KYC integrada',                                                                                                                     en: 'Integrated KYC verification'                                                                                                                     },
    premF4:          { es: 'Retiro a cuenta bancaria',                                                                                                                       en: 'Bank account withdrawal'                                                                                                                         },
    premCta:         { es: 'Ver quinielas premium',                                                                                                                          en: 'View premium pools'                                                                                                                              },
    // Tech highlights
    techEyebrow:     { es: 'Plataforma',                                                                                                                                     en: 'Platform'                                                                                                                                        },
    techTitle:       { es: 'Construido para el torneo',                                                                                                                      en: 'Built for the tournament'                                                                                                                        },
    tech1Title:      { es: 'Resultados en tiempo real',                                                                                                                      en: 'Real-time results'                                                                                                                               },
    tech1Desc:       { es: 'Marcadores sincronizados automáticamente desde la fuente oficial cada 30 segundos.',                                                             en: 'Scores updated automatically from the official source every 30 seconds.'                                                                        },
    tech2Title:      { es: 'Ranking competitivo',                                                                                                                            en: 'Competitive ranking'                                                                                                                             },
    tech2Desc:       { es: 'Tu posición se actualiza partido a partido. Escala el leaderboard con cada predicción acertada.',                                                en: 'Your position updates match by match. Climb the leaderboard with every correct prediction.'                                                    },
    tech3Title:      { es: 'Premios reales',                                                                                                                             en: 'Real prizes'                                                                                                                                     },
    tech3Desc:       { es: 'Retira directamente a tu cuenta bancaria local. Sin intermediarios.',                                                                            en: 'Withdraw directly to your local bank account. No middlemen.'                                                                                     },
    tech4Title:      { es: 'Torneo internacional',                                                                                                                           en: 'International tournament'                                                                                                                        },
    tech4Desc:       { es: '48 selecciones, 3 países sede (Canadá, México y USA), 1 campeón del mundo.',                                                                    en: '48 teams, 3 host countries (Canada, Mexico, and USA), 1 world champion.'                                                                        },
    // Final CTA
    ctaTitle1:       { es: '¿Listo para',                                                                                                                                    en: 'Ready to'                                                                                                                                        },
    ctaGold:         { es: 'predecir?',                                                                                                                                      en: 'predict?'                                                                                                                                        },
    ctaSubtitle:     { es: 'Únete antes del primer partido y no te pierdas ninguna predicción de la fase de grupos.',                                                        en: "Join before the first match and don't miss a single group stage prediction."                                                                   },
    ctaPrimary:      { es: 'Crear cuenta gratis',                                                                                                                            en: 'Create free account'                                                                                                                             },
    ctaSecondary:    { es: 'Explorar quinielas',                                                                                                                             en: 'Explore pools'                                                                                                                                   },
    // Logged-in user overrides — replaces sign-up CTAs for authenticated visitors
    loggedInHeroPrimary:   { es: 'Ir al tablero',                                                                                                                            en: 'Go to dashboard'                                                                                                                                 },
    loggedInHeroSecondary: { es: 'Mis predicciones',                                                                                                                         en: 'My predictions'                                                                                                                                  },
    loggedInFreeCta:       { es: 'Explorar quinielas',                                                                                                                       en: 'Explore pools'                                                                                                                                   },
    loggedInCtaTitle1:     { es: '¡Sigue',                                                                                                                                   en: 'Keep'                                                                                                                                            },
    loggedInCtaGold:       { es: 'prediciendo!',                                                                                                                             en: 'predicting!'                                                                                                                                     },
    loggedInCtaSubtitle:   { es: 'Revisa tus predicciones, sube en el ranking y compite por los premios.',                                                                   en: 'Check your predictions, climb the rankings, and compete for prizes.'                                                                             },
    loggedInCtaPrimary:    { es: 'Ver mi tablero',                                                                                                                           en: 'View my dashboard'                                                                                                                               },
    loggedInCtaSecondary:  { es: 'Mis quinielas',                                                                                                                            en: 'My pools'                                                                                                                                        },
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
    joinErrorPending: { es: 'Ya tienes una solicitud pendiente para este grupo. Espera a que un miembro te acepte.', en: 'You already have a pending join request. Wait for a member to accept you.' },
    joinErrorMember:  { es: 'Ya eres miembro activo de este grupo.',  en: 'You are already an active member of this group.' },
    joinErrorFull:    { es: 'El grupo alcanzó su límite de miembros gratuitos. El administrador debe actualizar a premium para agregar más miembros.', en: 'This group has reached its free-member limit. The admin must upgrade to premium to add more members.' },
    done:             { es: 'Listo',                            en: 'Done'                           },
    createBtn:        { es: 'Crear',                            en: 'Create'                         },
    joinBtn:          { es: 'Unirse',                           en: 'Join'                           },
    pendingTitle:          { es: 'Solicitudes pendientes',                                   en: 'Pending requests'                                          },
    noPending:             { es: 'No hay solicitudes pendientes.',                          en: 'No pending requests.'                                      },
    approve:               { es: 'Aceptar',                                                 en: 'Accept'                                                    },
    reject:                { es: 'Rechazar',                                                en: 'Reject'                                                    },
    pendingBadge:          { es: 'Pendiente',                                               en: 'Pending'                                                   },
    checkingAvailability:  { es: 'Verificando disponibilidad…',                             en: 'Checking availability…'                                    },
    nameAvailable:         { es: 'Nombre disponible',                                       en: 'Name available'                                            },
    nameTaken:             { es: 'Ya existe un grupo con ese nombre. Elige otro.',          en: 'A group with that name already exists. Please choose another.' },
    livePredictionsTitle:  { es: 'Predicciones en Vivo',                                    en: 'Live Predictions'                                              },
    livePredictionsEmpty:  { es: 'No hay partidos en vivo en este momento.',                en: 'No matches are currently in progress.'                         },
    noPrediction:          { es: 'Sin predicción',                                          en: 'No prediction'                                                 },
    predictionVs:          { es: 'vs',                                                      en: 'vs'                                                            },
    showPredictions:       { es: 'Ver predicciones de',                                      en: 'Show predictions for'                                          },
    hidePredictions:       { es: 'Ocultar predicciones de',                                 en: 'Hide predictions for'                                          },
    predCount:             { es: 'predicciones',                                             en: 'predictions'                                                   },
  },
  ledger: {
    credit:           { es: 'Crédito',              en: 'Credit'             },
    creditRecurrente: { es: 'Crédito · Recurrente', en: 'Credit · Recurrente' },
    creditPayPal:     { es: 'Crédito · PayPal',     en: 'Credit · PayPal'    },
    creditBank:       { es: 'Crédito · Banco',      en: 'Credit · Bank'      },
    withdrawal:       { es: 'Retiro',               en: 'Withdrawal'         },
    refund:           { es: 'Reembolso',            en: 'Refund'             },
    earning:          { es: 'Ganancia',             en: 'Earning'            },
    entryFee:         { es: 'Pago',                 en: 'Payment'            },
  },
  dashboard: {
    hello:              { es: 'Hola',                          en: 'Hello'                        },
    player:             { es: 'Jugador',                       en: 'Player'                       },
    subtitle:           { es: 'Centro de control personal',    en: 'Personal command center'      },
    kycTitle:           { es: 'Verifica tu identidad',         en: 'Verify your identity'         },
    kycCopy:            { es: 'Completa tu verificación de identidad para poder hacer retiros.', en: 'Complete your identity verification to unlock withdrawals.' },
    kycAction:          { es: 'Verificar ahora',               en: 'Verify now'                   },
    kycVerifiedTitle:   { es: 'Identidad verificada',          en: 'Identity verified'            },
    kycVerifiedCopy:    { es: 'Tu cuenta está verificada y tienes acceso completo a retiros.', en: 'Your account is verified and you have full access to withdrawals.' },
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
  balance: {
    title:               { es: 'BALANCE',                                                       en: 'BALANCE'                                                     },
    historyTitle:        { es: 'Historial de transacciones',                                    en: 'Transaction history'                                         },
    historyEmpty:        { es: 'Sin transacciones aún',                                         en: 'No transactions yet'                                         },
    bannerClose:              { es: 'Cerrar notificación',                                      en: 'Close notification'                                          },
    paypalBannerPre:          { es: 'Pago PayPal de',                                           en: 'PayPal payment of'                                           },
    paypalBannerReceived:     { es: 'recibido',                                                 en: 'received'                                                    },
    paypalBannerRate:         { es: 'al tipo de compra',                                        en: 'at the buy rate'                                             },
    paypalBannerCredit:       { es: 'La acreditación aparecerá en el historial en breve.',      en: 'The credit will appear in your history shortly.'             },
    recurrenteBannerPre:      { es: 'Pago con Recurrente de',                                   en: 'Recurrente payment of'                                       },
    recurrenteBannerReceived: { es: 'recibido',                                                 en: 'received'                                                    },
    recurrenteBannerCredit:   { es: 'La acreditación aparecerá en el historial en breve.',      en: 'The credit will appear in your history shortly.'             },
    // Unified transaction list (payment intents + bank transfers)
    txTitle:                  { es: 'Mis transacciones',                                        en: 'My transactions'                                             },
    txEmpty:                  { es: 'No has realizado ningún pago todavía.',                    en: 'No payments made yet.'                                       },
    providerTransfer:         { es: 'Transferencia bancaria',                                   en: 'Bank transfer'                                               },
    underReviewMsg:           { es: 'Disputa enviada — en revisión por el equipo',             en: 'Dispute submitted — under team review'                       },
    comprobanteRequiredMsg:   { es: 'El equipo solicita que adjuntes un comprobante',          en: 'The team is requesting a payment receipt'                    },
    btnAdjuntar:              { es: 'Adjuntar',                                                 en: 'Attach'                                                      },
    btnResolucion:            { es: 'Resolución',                                               en: 'Dispute'                                                     },
    btnCancel:                { es: 'Cancelar',                                                 en: 'Cancel'                                                      },
  },
  // ── Comprobante upload (pending intent receipt upload page) ───────────────────
  comprobante: {
    title:          { es: 'Adjuntar comprobante',                                                                                              en: 'Attach receipt'                                                                                                    },
    backLabel:      { es: 'Volver',                                                                                                            en: 'Back'                                                                                                              },
    backToBalance:  { es: 'Volver al balance',                                                                                                 en: 'Back to balance'                                                                                                   },
    subtitlePrefix: { es: 'Pago',                                                                                                              en: 'Payment'                                                                                                           },
    desc:           { es: 'Sube una imagen o PDF del comprobante de tu pago. El equipo revisará y acreditará el monto a tu balance.',           en: 'Upload an image or PDF of your payment receipt. The team will review and credit the amount to your balance.'        },
    clickToSelect:  { es: 'Haz clic para seleccionar',                                                                                         en: 'Click to select'                                                                                                   },
    dragHere:       { es: 'o arrastra aquí tu archivo',                                                                                        en: 'or drag your file here'                                                                                            },
    fileTypes:      { es: 'PNG, JPG, PDF — máx 10 MB',                                                                                        en: 'PNG, JPG, PDF — max 10 MB'                                                                                         },
    filePrefix:     { es: 'Archivo:',                                                                                                          en: 'File:'                                                                                                             },
    fileTooLarge:   { es: 'El archivo no puede superar los 10 MB.',                                                                            en: 'File must not exceed 10 MB.'                                                                                       },
    noFile:         { es: 'Selecciona un archivo antes de continuar.',                                                                         en: 'Please select a file before continuing.'                                                                           },
    successTitle:   { es: 'Comprobante enviado correctamente',                                                                                 en: 'Receipt submitted successfully'                                                                                    },
    successDesc:    { es: 'El equipo revisará tu comprobante y acreditará el monto a tu balance.',                                             en: 'The team will review your receipt and credit the amount to your balance.'                                           },
    submit:         { es: 'Enviar comprobante',                                                                                                en: 'Send receipt'                                                                                                      },
    submitting:     { es: 'Subiendo...',                                                                                                       en: 'Uploading...'                                                                                                      },
    notFound:       { es: 'Pago no encontrado o ya fue procesado.',                                                                            en: 'Payment not found or already processed.'                                                                           },
  },
  // ── Review request (rejected intent dispute form) ─────────────────────────────
  review: {
    title:          { es: 'Solicitar revisión',                                                                                                en: 'Request review'                                                                                                    },
    backLabel:      { es: 'Volver',                                                                                                            en: 'Back'                                                                                                              },
    backToBalance:  { es: 'Volver al balance',                                                                                                 en: 'Back to balance'                                                                                                   },
    subtitleStatus: { es: 'rechazado',                                                                                                         en: 'rejected'                                                                                                          },
    desc:           { es: 'Describe el problema y, si tienes evidencia adicional, adjunta un comprobante. El equipo revisará tu caso y actualizará el estado del pago.', en: 'Describe the problem and, if you have additional evidence, attach a receipt. The team will review your case and update the payment status.' },
    notesLabel:     { es: 'Descripción del problema',                                                                                          en: 'Problem description'                                                                                               },
    notesPh:        { es: 'Ej: El pago fue procesado correctamente en mi cuenta pero no fue acreditado. Adjunto captura de pantalla de la transacción...', en: 'E.g. The payment was processed in my account but was not credited. I am attaching a screenshot of the transaction...' },
    notesMinChars:  { es: 'Mínimo 10 caracteres',                                                                                              en: 'Minimum 10 characters'                                                                                             },
    fileLabel:      { es: 'Comprobante adicional',                                                                                             en: 'Additional receipt'                                                                                                },
    fileOptional:   { es: '(opcional)',                                                                                                         en: '(optional)'                                                                                                        },
    clickToSelect:  { es: 'Haz clic o arrastra un archivo',                                                                                    en: 'Click or drag a file here'                                                                                         },
    fileTypes:      { es: 'PNG, JPG, PDF — máx 10 MB',                                                                                        en: 'PNG, JPG, PDF — max 10 MB'                                                                                         },
    filePrefix:     { es: 'Archivo:',                                                                                                          en: 'File:'                                                                                                             },
    fileTooLarge:   { es: 'El archivo no puede superar los 10 MB.',                                                                            en: 'File must not exceed 10 MB.'                                                                                       },
    successTitle:   { es: 'Solicitud de revisión enviada',                                                                                     en: 'Review request submitted'                                                                                          },
    successDesc:    { es: 'El equipo revisará tu evidencia y se pondrá en contacto contigo. El estado cambiará a "En revisión".',               en: 'The team will review your evidence and contact you. The status will change to "Under review".'                    },
    submit:         { es: 'Enviar solicitud de revisión',                                                                                      en: 'Submit review request'                                                                                             },
    submitting:     { es: 'Enviando...',                                                                                                        en: 'Sending...'                                                                                                        },
    notFound:       { es: 'Pago no encontrado o no disponible para resolución.',                                                               en: 'Payment not found or not available for dispute.'                                                                   },
  },
  balanceCard: {
    title:    { es: 'Balance',    en: 'Balance'  },
    available:{ es: 'Disponible', en: 'Available'},
    reserved: { es: 'Reservado',  en: 'Reserved' },
    pending:  { es: 'Pendiente',  en: 'Pending'  },
    deposit:  { es: 'Depositar',  en: 'Deposit'  },
    withdraw: { es: 'Retirar',    en: 'Withdraw' },
  },
  deposit: {
    label:                  { es: 'Depósito',                                                             en: 'Deposit'                                                          },
    tabRecurrente:           { es: 'Recurrente',                                                          en: 'Recurrente'                                                       },
    tabPaypal:               { es: 'PayPal',                                                              en: 'PayPal'                                                           },
    recurrenteDesc:          { es: 'Deposita con tarjeta de débito/crédito guatemalteca vía Recurrente. Serás redirigido al portal de pago.', en: 'Deposit with a Guatemalan debit/credit card via Recurrente. You will be redirected to the payment portal.' },
    amountGTQLabel:          { es: 'Monto (GTQ)',                                                         en: 'Amount (GTQ)'                                                     },
    recurrenteContinue:      { es: 'Continuar con Recurrente',                                            en: 'Continue with Recurrente'                                         },
    recurrenteSessionExpired:{ es: 'Sesión expirada. Inicia sesión nuevamente antes de continuar.',       en: 'Session expired. Sign in again before continuing.'                },
    recurrenteUnavailable:   { es: 'El pago con Recurrente no está disponible en este momento. Intenta más tarde o usa otro método.', en: 'Recurrente payments are not available right now. Try again later or use another method.' },
    recurrenteError:         { es: 'No pudimos iniciar el pago con Recurrente. Intenta de nuevo.',         en: 'We could not start the Recurrente payment. Please try again.'     },
    paypalDesc:              { es: 'Deposita con tu cuenta PayPal. Se usa la tasa de compra del día.',    en: "Deposit with your PayPal account. Today's buy rate is used."        },
    buyRateLabel:            { es: 'Tasa compra:',                                                        en: 'Buy rate:'                                                        },
    amountUSDLabel:          { es: 'Monto (USD)',                                                         en: 'Amount (USD)'                                                     },
    paypalNotConfiguredPre:  { es: 'PayPal no está configurado. Agrega',                                  en: 'PayPal is not configured. Add'                                    },
    paypalNotConfiguredPost: { es: 'al entorno.',                                                          en: 'to the environment.'                                              },
    paypalSessionExpired:    { es: 'Sesión expirada. Inicia sesión nuevamente antes de pagar con PayPal.', en: 'Session expired. Sign in again before paying with PayPal.'        },
    paypalNoOrderId:         { es: 'PayPal no devolvió un ID de orden válido.',                            en: 'PayPal did not return a valid order ID.'                          },
    paypalCancelled:         { es: 'Pago cancelado.',                                                      en: 'Payment cancelled.'                                               },
    paypalError:             { es: 'Error al procesar el pago con PayPal.',                                en: 'Error processing the PayPal payment.'                             },
    paypalServiceUnavailable:{ es: 'El pago con PayPal no está disponible en este momento. Intenta más tarde o usa otro método.', en: 'PayPal payments are not available right now. Try again later or use another method.' },
    paypalUpstreamError:     { es: 'No pudimos completar el pago con PayPal. Intenta de nuevo.',           en: 'We could not complete the PayPal payment. Please try again.'      },
    paypalInvalidRequest:    { es: 'Monto o moneda inválidos.',                                            en: 'Invalid amount or currency.'                                      },
    bankImageAlt:        { es: 'Datos para transferencia bancaria: Eder Yafeth Garcia Quiroa, cuenta monetaria en Banco Industrial y BAC.', en: 'Bank transfer details: Eder Yafeth Garcia Quiroa, checking account at Banco Industrial and BAC.' },
    bankNoteGT:          { es: 'Aplica únicamente para transferencias realizadas dentro de Guatemala.',                                      en: 'Applies only to bank transfers made within Guatemala.'                                          },
    bankHelp:            { es: 'Deposita al banco y adjunta el comprobante. La revisión toma 24-48 horas.',                                  en: 'Make the bank transfer and attach the receipt. Review takes 24-48 hours.'                       },
    tabBank:             { es: 'Transferencia',                                                                                               en: 'Bank Transfer'                                                                                  },
    bankAmountLabel:     { es: 'Monto depositado (GTQ)',                                                                                      en: 'Amount deposited (GTQ)'                                                                         },
    bankFileLabel:       { es: 'Comprobante (JPEG, PNG, WebP, PDF — máx. 5 MB)',                                                             en: 'Receipt (JPEG, PNG, WebP, PDF — max. 5 MB)'                                                    },
    bankFileButton:      { es: 'Toca para adjuntar',                                                                                          en: 'Tap to attach'                                                                                  },
    bankSubmit:          { es: 'Enviar comprobante',                                                                                          en: 'Send receipt'                                                                                   },
    bankSuccess:         { es: 'Comprobante enviado correctamente. La revisión toma entre 24 y 48 horas hábiles.',                            en: 'Receipt submitted successfully. Review takes between 24 and 48 business hours.'                 },
    bankFileTooBig:      { es: 'El archivo no debe superar 5 MB.',                                                                            en: 'File must not exceed 5 MB.'                                                                     },
    bankFileInvalidType: { es: 'Tipo de archivo no permitido. Usa JPEG, PNG, WebP o PDF.',                                                   en: 'File type not permitted. Use JPEG, PNG, WebP or PDF.'                                          },
    bankFileNoAttach:    { es: 'Adjunta el comprobante antes de continuar.',                                                                  en: 'Please attach the receipt before continuing.'                                                   },
  },
  withdraw: {
    title:             { es: 'RETIRAR',                                                          en: 'WITHDRAW'                                                              },
    available:         { es: 'Disponible para retirar',                                          en: 'Available to withdraw'                                                 },
    method:            { es: 'Método de retiro',                                                 en: 'Withdrawal method'                                                     },
    methodBankGT:      { es: 'Depósito Bancario',                                                en: 'Bank Deposit'                                                          },
    methodPayPal:      { es: 'PayPal',                                                           en: 'PayPal'                                                                },
    amount:            { es: 'Monto a retirar (GTQ)',                                            en: 'Amount to withdraw (GTQ)'                                              },
    amountUSD:         { es: 'Monto a retirar (USD)',                                            en: 'Amount to withdraw (USD)'                                              },
    amountExceeds:     { es: 'Excede el balance disponible',                                     en: 'Exceeds available balance'                                             },
    amountBelowMin:    { es: 'El monto mínimo de retiro es',                                     en: 'Minimum withdrawal amount is'                                          },
    bankSelect:        { es: 'Institución Financiera',                                           en: 'Financial Institution'                                                 },
    bankSelectPh:      { es: 'Selecciona una institución',                                       en: 'Select an institution'                                                 },
    bankSelectLoading: { es: 'Cargando instituciones…',                                          en: 'Loading institutions…'                                                 },
    accountType:       { es: 'Tipo de cuenta',                                                   en: 'Account type'                                                          },
    accountTypePh:     { es: 'Selecciona un tipo de cuenta',                                     en: 'Select an account type'                                                },
    accountTypeLoading:{ es: 'Cargando tipos…',                                                  en: 'Loading types…'                                                        },
    accountNumber:     { es: 'Número de cuenta',                                                 en: 'Account number'                                                        },
    accountNumberPh:   { es: '000-000000-0',                                                     en: '000-000000-0'                                                          },
    accountHolder:     { es: 'Titular de la cuenta',                                             en: 'Account holder'                                                        },
    accountHolderPh:   { es: 'Nombre completo',                                                  en: 'Full name'                                                             },
    paypalEmail:       { es: 'Correo PayPal',                                                    en: 'PayPal email'                                                          },
    paypalEmailPh:     { es: 'tu@email.com',                                                     en: 'you@email.com'                                                         },
    submit:            { es: 'Retirar',                                                          en: 'Withdraw'                                                              },
    successMsg:        { es: 'Solicitud de retiro enviada. Será procesada en 1–3 días hábiles.',                          en: 'Withdrawal request submitted. It will be processed in 1–3 business days.'        },
    successTitle:      { es: 'Solicitud enviada',                                                                           en: 'Request submitted'                                                               },
    errorTitle:        { es: 'No se pudo procesar',                                                                        en: 'Could not process'                                                               },
    errorGeneric:      { es: 'No pudimos procesar tu solicitud. Intenta de nuevo o contacta a soporte si el problema persiste.', en: 'We could not process your request. Please try again or contact support if the problem persists.' },
    errorClose:        { es: 'Cerrar',                                                                                     en: 'Close'                                                                           },
    errorRetry:        { es: 'Intentar de nuevo',                                                                          en: 'Try again'                                                                       },
    kycRequired:        { es: 'Verificación requerida',                                                                                                                                                                                                                                            en: 'Verification required'                                                                                                                                                                                                                                                              },
    kycRequiredDesc:    { es: 'Para retirar fondos debes completar la verificación de identidad. Los residentes en Guatemala deben enviar su DPI vigente; los extranjeros deben enviar un documento de identidad oficial vigente (pasaporte, cédula de residencia u equivalente).', en: 'To withdraw funds you must complete your identity verification. Guatemala residents must submit a valid DPI; foreigners must submit a valid official identity document (passport, residence card, or equivalent).' },
    kycRequiredAction:  { es: 'Sube tus documentos en la sección Verificación de Identidad y espera la aprobación del equipo de cumplimiento.',                                                                                                                                    en: 'Upload your documents in the Identity Verification section and await approval from the compliance team.'                                                                                                     },
    kycAction:          { es: 'Ir a Verificación de Identidad',                                                                                                                                                                                                                    en: 'Go to Identity Verification'                                                                                                                                                                                },
  },
  tournaments: {
    title:      { es: 'Fan Fest',                                              en: 'Fan Fest'                                 },
    subtitle:   { es: 'Revisa la clasificación de tus amigos, apoya al favorito y sigue los partidos en vivo.', en: "Check your friends' standings, back your favourite, and follow the matches live." },
    emptyTitle: { es: 'No hay torneos disponibles',                            en: 'No tournaments available'                 },
    emptyDesc:  { es: 'Vuelve pronto para ver las proximas kinielas',         en: 'Check back soon for upcoming pools'       },
    prize:      { es: 'Pozo',                                                  en: 'Prize pool'                               },
    entry:      { es: 'Inscripcion',                                           en: 'Entry'                                    },
    enter:      { es: 'Entrar',                                                en: 'Enter'                                    },
    details:    { es: 'Ver detalles',                                          en: 'View details'                             },
    members:    { es: 'miembros',                                              en: 'members'                                  },
    createPick: { es: 'Crear kiniela',                                         en: 'Create kiniela'                          },
    marketplace:      { es: 'Kinielas destacadas',         en: 'Featured pools'           },
    availablePools:   { es: 'Pools disponibles',           en: 'Available pools'          },
    options:          { es: 'opciones',                    en: 'options'                  },
    myPools:          { es: 'Mis kinielas',                en: 'My pools'                 },
    featuredPools:    { es: 'Destacadas',                  en: 'Featured'                 },
    noFeatured:       { es: 'Aún no tienes kinielas destacadas. Marca una con ★ para verla aquí.', en: 'No featured pools yet. Star one to see it here.' },
    addFeatured:      { es: 'Marcar como destacada',       en: 'Mark as featured'         },
    removeFeatured:   { es: 'Quitar de destacadas',        en: 'Remove from featured'     },
    viewDetail:       { es: 'Ver detalle',                 en: 'View detail'              },
    joinPool:         { es: 'Unirse a kiniela',            en: 'Join pool'                },

    // Group-code lookup widget
    lookupTitle:         { es: 'Ver clasificación de grupo',                                        en: 'View group standings'                                       },
    lookupDesc:          { es: 'Ingresa el código de un grupo para visualizar su clasificación actual.', en: 'Enter a group code to view its current standings.'      },
    lookupPlaceholder:   { es: 'Código del grupo',                                                  en: 'Group code'                                                 },
    lookupBtn:           { es: 'Ver',                                                               en: 'View'                                                       },
    lookupClose:         { es: 'Cerrar clasificación',                                              en: 'Close standings'                                            },
    lookupCurrentLabel:  { es: 'Clasificación Actual',                                              en: 'Current Standings'                                          },
    lookupColPlayer:     { es: 'Jugador',                                                           en: 'Player'                                                     },
    lookupSubs:          { es: 'Suplentes',                                                         en: 'Substitutes'                                                },
    lookupEmpty:         { es: 'Aún no hay puntuaciones registradas para este grupo.',              en: 'No scores recorded for this group yet.'                     },
    lookupErrorNotFound: { es: 'No se encontró ningún grupo con ese código.',                       en: 'No group found with that code.'                             },
    lookupErrorGeneric:  { es: 'Error al buscar el grupo.',                                         en: 'Error looking up the group.'                                },
    lookupErrorNetwork:  { es: 'No se pudo conectar al servidor. Intenta de nuevo.',                en: 'Could not connect to the server. Please try again.'         },

    // Live match feed widget
    liveTitle:         { es: 'Partidos de hoy',                                    en: "Today's Matches"                                     },
    liveBadge:         { es: 'EN VIVO',                                            en: 'LIVE'                                                 },
    liveInPlay:        { es: 'En juego',                                           en: 'In play'                                              },
    liveHT:            { es: 'MT',                                                 en: 'HT'                                                   },
    liveEmpty:         { es: 'No hay partidos programados para hoy.',              en: 'No matches scheduled for today.'                      },
    liveError:         { es: 'No se pudo cargar la información de partidos.',      en: 'Could not load match information.'                    },
    liveHalftime:      { es: 'Medio tiempo',                                       en: 'Half time'                                            },
    liveEvents:        { es: 'Eventos',                                            en: 'Events'                                               },
    liveNoEvents:      { es: 'Sin eventos aún.',                                   en: 'No events yet.'                                       },
    liveLineups:       { es: 'Alineaciones',                                       en: 'Lineups'                                              },
    liveNoData:        { es: 'Información del partido aún no disponible.',         en: 'Match information not yet available.'                 },
    liveSubs:          { es: 'Suplentes',                                          en: 'Substitutes'                                          },
  },
  predictions: {
    title:        { es: 'Panel de predicciones',     en: 'Prediction panel'          },
    subtitle:     { es: 'Lista de predicciones por partido. Filtra por grupo y guarda marcadores antes del inicio.', en: 'Match-by-match prediction list. Filter by group and save scores before kick-off.' },
    filterAll:    { es: 'Todos',                     en: 'All'                        },
    filterPending:{ es: 'Pendientes',                en: 'Pending'                    },
    filterSaved:  { es: 'Guardados',                 en: 'Saved'                      },
    filterPast:   { es: 'Pasados',                   en: 'Past'                       },
    viewByGroup:  { es: 'Grupos',                    en: 'Groups'                     },
    viewByDay:    { es: 'Partidos',                  en: 'Matches'                    },
    groupSelector:{ es: 'Grupos',                    en: 'Groups'                     },
    groupSelectorAll:{ es: 'Mostrando partidos de todos los grupos', en: 'Showing matches from all groups' },
    groupSelected:{ es: 'Mostrando Grupo',           en: 'Showing Group'              },
    groupAll:     { es: 'Todos',                     en: 'All'                        },
    matches:      { es: 'partidos',                  en: 'matches'                    },
    noMatches:             { es: 'Sin partidos en este grupo',                           en: 'No matches in this group'                             },
    noMatchesDesc:         { es: 'Este grupo no tiene partidos registrados todavía.',     en: 'No matches have been registered for this group yet.'  },
    noMatchesToday:        { es: 'Sin partidos para hoy',                                 en: 'No matches today'                                     },
    noMatchesTodayDesc:    { es: 'No hay encuentros programados para el día de hoy. Vuelve mañana para capturar tus predicciones.', en: 'There are no fixtures scheduled for today. Come back tomorrow to submit your predictions.' },
    noMatchesForDate:      { es: 'Sin partidos para esta fecha',                          en: 'No matches for this date'                             },
    noMatchesForDateDesc:  { es: 'No hay partidos programados para la fecha seleccionada.', en: 'No matches are scheduled for the selected date.'    },
    noMatchesForRange:     { es: 'Sin partidos en el rango seleccionado',                 en: 'No matches in selected range'                         },
    noMatchesForRangeDesc: { es: 'No hay partidos en las fechas seleccionadas.',          en: 'No matches scheduled in the selected date range.'     },
    calendarDayHdrs:       { es: 'L,M,X,J,V,S,D',                                       en: 'Mo,Tu,We,Th,Fr,Sa,Su'                                 },
    calendarPrevMonth:     { es: 'Mes anterior',                                         en: 'Previous month'                                       },
    calendarNextMonth:     { es: 'Mes siguiente',                                        en: 'Next month'                                           },
    calendarRangeHint:     { es: 'Selecciona la fecha final del rango',                  en: 'Click a second date to select a range'                },
    calendarGoToday:       { es: 'Partidos de hoy',                                     en: "Today's matches"                                      },
    calendarToggle:        { es: 'Abrir calendario',                                    en: 'Open calendar'                                        },
    pagePrev:              { es: 'Página anterior',                                      en: 'Previous page'                                        },
    pageNext:              { es: 'Página siguiente',                                     en: 'Next page'                                            },
    allSavedToday:         { es: 'Todas las predicciones del día guardadas',              en: 'All predictions for today saved'                      },
    allSavedTodayDesc:     { es: '¡Buen trabajo! Ya tienes marcadores guardados para todos los partidos de hoy.', en: 'Great work! You have already saved your scores for every match today.' },
    noPredictionsToday:    { es: 'Aún no tienes predicciones guardadas hoy',             en: 'No predictions saved yet today'                       },
    noPredictionsTodayDesc:{ es: 'Guarda tus predicciones para los partidos de hoy antes de que inicien.', en: 'Save your predictions for today\'s matches before they kick off.' },
    noPastMatches:         { es: 'Sin partidos finalizados',                                              en: 'No finished matches'                                          },
    noPastMatchesDesc:     { es: 'Los partidos finalizados aparecerán aquí con tu puntuación obtenida.', en: 'Finished matches will appear here along with your score.'      },
    liveLabel:             { es: 'EN VIVO',                                                               en: 'LIVE'                                                         },
    loadError:    { es: 'No se pudieron cargar los partidos', en: 'Could not load matches' },
    loadErrorDesc:{ es: 'Intenta recargar la pagina. Si el problema persiste, contacta al administrador.', en: 'Try reloading the page. If the problem persists, contact the administrator.' },
    kickoff:      { es: 'Inicio',                    en: 'Kickoff'                    },
    timezone:     { es: 'Hora local',                en: 'Local time'                 },
    phase:        { es: 'Fase',                      en: 'Phase'                      },
    venue:        { es: 'Sede',                      en: 'Venue'                      },
    saved:        { es: 'Guardado',                  en: 'Saved'                      },
    unsaved:      { es: 'Sin guardar',               en: 'Unsaved'                    },
    locked:       { es: 'Bloqueado',                 en: 'Locked'                     },
    pendingSync:  { es: 'Iniciando...',              en: 'Starting...'                },
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
  bracket:           { es: 'Llave',                                en: 'Bracket'                                },
  bracketTitle:      { es: 'Llave del Torneo',                     en: 'Tournament Bracket'                     },
  bracketNoSlots:    { es: 'La llave se irá llenando conforme avancen las rondas.', en: 'The bracket will fill in as rounds progress.' },
  phaseRoundOf16:    { es: 'Octavos',                              en: 'Round of 16'                            },
  phaseQuarterFinal: { es: 'Cuartos',                              en: 'Quarter-finals'                         },
  phaseSemiF:        { es: 'Semis',                                en: 'Semi-finals'                            },
  phaseThirdPlace:   { es: '3.er Lugar',                           en: '3rd Place'                              },
  phaseFinal:        { es: 'Final',                                en: 'Final'                                  },
  // ── Group standings (public landing page) ────────────────────────────────────
  standings: {
    eyebrow:          { es: 'Copa del Mundo FIFA 2026',                                        en: 'FIFA World Cup 2026'                                           },
    title:            { es: 'Tabla de posiciones',                                             en: 'Group standings'                                               },
    subtitle:         { es: 'Posiciones actualizadas en tiempo real según los resultados registrados.', en: 'Standings updated in real time from recorded results.' },
    group:            { es: 'Grupo',                                                           en: 'Group'                                                         },
    team:             { es: 'Equipo',                                                          en: 'Team'                                                          },
    played:           { es: 'J',                                                               en: 'P'                                                             },
    won:              { es: 'G',                                                               en: 'W'                                                             },
    drawn:            { es: 'E',                                                               en: 'D'                                                             },
    lost:             { es: 'P',                                                               en: 'L'                                                             },
    gf:               { es: 'GF',                                                              en: 'GF'                                                            },
    ga:               { es: 'GC',                                                              en: 'GA'                                                            },
    pts:              { es: 'Pts',                                                             en: 'Pts'                                                           },
    legendAdvanced:   { es: 'Clasificado',                                                     en: 'Advancing'                                                     },
    legendMaybe:      { es: 'Puede clasificar',                                                en: 'May advance'                                                   },
    legendEliminated: { es: 'Eliminado',                                                       en: 'Eliminated'                                                    },
    error:            { es: 'No se pudieron cargar las posiciones.',                           en: 'Could not load standings.'                                     },
    empty:            { es: 'Los partidos de grupos aún no han comenzado.',                    en: 'Group stage matches have not started yet.'                     },
  },
  ticker: {
    live:     { es: 'Tipo de cambio en tiempo real', en: 'Live exchange rate'      },
    outdated: { es: 'tipo de cambio desactualizado', en: 'exchange rate is stale'  },
  },
  // prettier-ignore
  kyc: {
    title:             { es: 'VERIFICACIÓN DE IDENTIDAD',    en: 'IDENTITY VERIFICATION'                    },
    // stepper labels
    stepUnverified:    { es: 'Sin verificar',                en: 'Unverified'                               },
    stepPending:       { es: 'Enviado',                      en: 'Submitted'                                },
    stepUnderReview:   { es: 'En revisión',                  en: 'Under review'                             },
    stepApproved:      { es: 'Aprobado',                     en: 'Approved'                                 },
    rejectionReason:   { es: 'Motivo de rechazo:',           en: 'Rejection reason:'                        },
    // under-review banner
    reviewBannerTitle: { es: 'Documentos en revisión',       en: 'Documents under review'                   },
    reviewBannerBody:  { es: 'Hemos recibido tus documentos y están siendo revisados por nuestro equipo. Te notificaremos cuando se complete la verificación.', en: 'We have received your documents and they are being reviewed by our team. We will notify you once verification is complete.' },
    // profile form
    profileTitle:      { es: 'Información personal',         en: 'Personal information'                     },
    submitProfile:     { es: 'Enviar información',           en: 'Submit information'                       },
    fieldFullName:     { es: 'Nombre completo',              en: 'Full name'                                },
    fieldDob:          { es: 'Fecha de nacimiento',          en: 'Date of birth'                            },
    fieldNationality:  { es: 'Nacionalidad',                 en: 'Nationality'                              },
    fieldDocNumber:    { es: 'Número de documento',          en: 'Document number'                          },
    fieldAddress:      { es: 'Dirección',                    en: 'Address'                                  },
    fieldCity:         { es: 'Ciudad',                       en: 'City'                                     },
    fieldPostalCode:   { es: 'Código postal',                en: 'Postal code'                              },
    phFullName:        { es: 'Nombre Apellido',              en: 'First Last'                               },
    phNationality:     { es: 'Guatemala',                    en: 'United Kingdom'                           },
    phDocNumber:       { es: '0000 00000 0000',              en: '0000 00000 0000'                          },
    phAddress:         { es: 'Calle, No.',                   en: 'Street, No.'                              },
    phCity:            { es: 'Ciudad de Guatemala',          en: 'City'                                     },
    phPostalCode:      { es: '01001',                        en: 'A00 0AA'                                  },
    // document upload
    docsTitle:         { es: 'Documentos',                   en: 'Documents'                                },
    docsHintA:         { es: 'Selecciona los archivos — se precargan localmente sin enviarse. Cuando ambos estén listos, presiona', en: 'Select your files — they are staged locally without being sent. Once both are ready, press' },
    docsHintB:         { es: 'para guardarlos. JPEG, PNG, WebP o PDF — máx. 10 MB por archivo.', en: 'to save them. JPEG, PNG, WebP or PDF — max. 10 MB per file.' },
    uploadBtn:         { es: 'Cargar archivos',              en: 'Upload files'                             },
    docGovId:          { es: 'DPI / Pasaporte (frente)',     en: 'ID / Passport (front)'                    },
    docSelfie:         { es: 'Selfie con DPI',               en: 'Selfie with ID'                           },
    docGovIdShort:     { es: 'DPI / Pasaporte',              en: 'ID / Passport'                            },
    docSelfieShort:    { es: 'Selfie con DPI',               en: 'Selfie with ID'                           },
    pendingLabel:      { es: 'Pendiente de subir',           en: 'Pending upload'                           },
    // validating / uploading state
    uploadingTitle:    { es: 'Enviando documentos...',       en: 'Uploading documents...'                   },
    uploadingBody:     { es: 'Por favor espera mientras se suben tus archivos.', en: 'Please wait while your files are being uploaded.' },
    validatingTitle:   { es: 'Validación en progreso',       en: 'Validation in progress'                   },
    validatingBody:    { es: 'Hemos recibido tus documentos. Te notificaremos cuando se complete la verificación.', en: 'We have received your documents. We will notify you once verification is complete.' },
    // errors
    errMaxSize:        { es: 'Máx. 10 MB por documento',    en: 'Max. 10 MB per document'                  },
    errFileType:       { es: 'Tipo no permitido. Usa JPEG, PNG, WebP o PDF.', en: 'File type not allowed. Use JPEG, PNG, WebP, or PDF.' },
    errUpload:         { es: 'Error al enviar documentos.',  en: 'Error uploading documents.'               },
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
  accountTypeName: (name: string | null | undefined) => string;
  slotDesc: (desc: string | null | undefined) => string;
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

// ISO 3166-1 alpha-2 codes for Spanish-speaking countries.
const SPANISH_COUNTRIES = new Set([
  "AR",
  "BO",
  "CL",
  "CO",
  "CR",
  "CU",
  "DO",
  "EC",
  "ES",
  "GQ",
  "GT",
  "HN",
  "MX",
  "NI",
  "PA",
  "PE",
  "PR",
  "PY",
  "SV",
  "UY",
  "VE",
]);

function localeForCountry(country: string): Locale {
  return SPANISH_COUNTRIES.has(country.toUpperCase()) ? "es" : "en";
}

// Translates Spanish bracket slot descriptions stored in the DB to the active
// locale. Four patterns exist (from migration 000205):
//   "N.° Grupo X"        → "Nth Group X"
//   "Mejor 3.° (p.N)"   → "Best 3rd (p.N)"
//   "Ganador PHASE MNN"  → "Winner PHASE MNN"
//   "Perdedor PHASE MNN" → "Loser PHASE MNN"
function englishOrdinal(n: number): string {
  if (n === 1) return "1st";
  if (n === 2) return "2nd";
  if (n === 3) return "3rd";
  return `${n}th`;
}

function translateSlotDescription(
  desc: string | null | undefined,
  locale: Locale,
): string {
  if (!desc) return "—";
  if (locale === "es") return desc;

  const groupMatch = /^(\d+)\.° Grupo ([A-L])$/.exec(desc);
  if (groupMatch) {
    return `${englishOrdinal(Number(groupMatch[1]))} Group ${groupMatch[2]}`;
  }

  const best3rd = /^Mejor 3\.° \(p\.(\d+)\)$/.exec(desc);
  if (best3rd) return `Best 3rd (p.${best3rd[1]})`;

  const winner = /^Ganador (.+)$/.exec(desc);
  if (winner) return `Winner ${winner[1]}`;

  const loser = /^Perdedor (.+)$/.exec(desc);
  if (loser) return `Loser ${loser[1]}`;

  return desc;
}

export function I18nProvider({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const [locale, setLocale] = useState<Locale>("es");
  const [timeZone, setTimeZone] = useState("UTC");

  useEffect(() => {
    setTimeZone(
      Intl.DateTimeFormat().resolvedOptions().timeZone || "America/Guatemala",
    );

    // Only skip geo detection when the user has explicitly chosen a language.
    // Auto-detected locales are not persisted so that geo data is re-evaluated
    // on each session (prevents stale/wrong country from being locked in forever).
    const storedSource = globalThis.localStorage.getItem(
      "quiniela-locale-source",
    );
    const storedLocale = globalThis.localStorage.getItem("quiniela-locale");
    if (
      storedSource === "explicit" &&
      (storedLocale === "es" || storedLocale === "en")
    ) {
      setLocale(storedLocale);
      document.documentElement.lang = storedLocale;
      return;
    }

    // No explicit preference — detect from connection country on every session.
    void (async () => {
      try {
        const r = await fetch("/api/geo", { cache: "no-store" });
        const data = (await r.json()) as { country?: string };
        const detected = localeForCountry(data.country ?? "GT");
        setLocale(detected);
        document.documentElement.lang = detected;
      } catch {
        // Geo lookup failed or not available (e.g. test env) — keep default "es".
      }
    })();
  }, []);

  const value = useMemo<I18nContextValue>(
    () => ({
      locale,
      timeZone,
      setLocale: (nextLocale) => {
        setLocale(nextLocale);
        globalThis.localStorage.setItem("quiniela-locale", nextLocale);
        globalThis.localStorage.setItem("quiniela-locale-source", "explicit");
        document.documentElement.lang = nextLocale;
      },
      t: (key) =>
        getTranslation(key, locale) ?? getTranslation(key, "es") ?? key,
      formatKickoff: (iso) => formatLocalizedDateTime(iso, locale, timeZone),
      phaseName: (phase) =>
        translateDictionaryValue(phase, phaseTranslations, locale),
      teamName: (name) =>
        translateDictionaryValue(name, teamTranslations, locale),
      accountTypeName: (name) =>
        translateDictionaryValue(name, accountTypeTranslations, locale),
      slotDesc: (desc) => translateSlotDescription(desc, locale),
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

package service

import (
	"context"
	"sort"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// tournamentService is the concrete implementation of TournamentService.
type tournamentService struct {
	matchRepo      repository.MatchRepository
	tournamentRepo repository.TournamentRepository
	log            *zap.Logger
}

// NewTournamentService constructs a tournamentService.
func NewTournamentService(
	matchRepo repository.MatchRepository,
	tournamentRepo repository.TournamentRepository,
	log *zap.Logger,
) TournamentService {
	return &tournamentService{
		matchRepo:      matchRepo,
		tournamentRepo: tournamentRepo,
		log:            log,
	}
}

// GetAllStandings returns real-time standings for every group that has at
// least one match scheduled. The map key is the group label ("A"–"L").
// Points are accumulated only from finished matches; scheduled/live matches
// are included to show teams with 0 points before their first result.
func (s *tournamentService) GetAllStandings(ctx context.Context) (map[string][]*domain.GroupStanding, error) {
	matches, err := s.matchRepo.ListByPhase(ctx, domain.PhaseGroupStage)
	if err != nil {
		return nil, err
	}
	return buildStandings(matches), nil
}

// GetGroupStanding returns real-time standings for a single group.
// Returns Validation when group is empty or NotFound when the group has no
// matches registered.
func (s *tournamentService) GetGroupStanding(ctx context.Context, group string) ([]*domain.GroupStanding, error) {
	if group == "" {
		return nil, apperrors.Validation("group label is required")
	}
	matches, err := s.matchRepo.ListByPhase(ctx, domain.PhaseGroupStage)
	if err != nil {
		return nil, err
	}
	all := buildStandings(matches)
	entries, ok := all[group]
	if !ok {
		return nil, apperrors.NotFound("group " + group + " not found")
	}
	return entries, nil
}

// CreateSlot creates a new bracket position slot. Only the system administrator
// may call this; the admin gate is enforced at the HTTP layer.
// Returns Validation when label is empty.
func (s *tournamentService) CreateSlot(ctx context.Context, label string) (*domain.TournamentSlot, error) {
	if label == "" {
		return nil, apperrors.Validation("slot label is required")
	}
	return s.tournamentRepo.CreateSlot(ctx, label)
}

// ConfirmSlot records the advancing team for a bracket slot.
// Returns Validation when team is empty; NotFound when the slot does not exist.
func (s *tournamentService) ConfirmSlot(ctx context.Context, slotID, adminID int, team string) (*domain.TournamentSlot, error) {
	if team == "" {
		return nil, apperrors.Validation("team name is required")
	}
	return s.tournamentRepo.ConfirmSlot(ctx, slotID, adminID, team)
}

// ListSlots returns all bracket position slots.
func (s *tournamentService) ListSlots(ctx context.Context) ([]*domain.TournamentSlot, error) {
	return s.tournamentRepo.ListSlots(ctx)
}

// buildStandings computes group standings from a slice of group-stage matches.
// All teams that appear in any match (regardless of status) are included so
// that teams with 0 finished matches still appear with zero stats.
// Points and win/draw/loss counts are accumulated only from finished matches
// that have non-nil scores.
func buildStandings(matches []*domain.Match) map[string][]*domain.GroupStanding {
	// standing accumulates per-team stats keyed by group+team.
	type key struct{ group, team string }
	acc := make(map[key]*domain.GroupStanding)

	ensure := func(group, team string) *domain.GroupStanding {
		k := key{group, team}
		if _, ok := acc[k]; !ok {
			acc[k] = &domain.GroupStanding{Group: group, Team: team}
		}
		return acc[k]
	}

	for _, m := range matches {
		if m.GroupLabel == nil {
			continue
		}
		g := *m.GroupLabel

		// Register both teams so they appear even before playing.
		ensure(g, m.HomeTeam)
		ensure(g, m.AwayTeam)

		if m.Status != domain.MatchStatusFinished || m.HomeScore == nil || m.AwayScore == nil {
			continue
		}

		home := ensure(g, m.HomeTeam)
		away := ensure(g, m.AwayTeam)
		hs, as := *m.HomeScore, *m.AwayScore

		home.Played++
		away.Played++
		home.GF += hs
		home.GC += as
		away.GF += as
		away.GC += hs

		switch {
		case hs > as:
			home.Won++
			home.Points += 3
			away.Lost++
		case hs < as:
			away.Won++
			away.Points += 3
			home.Lost++
		default:
			home.Drawn++
			home.Points++
			away.Drawn++
			away.Points++
		}
	}

	// Compute goal difference and group into result map.
	grouped := make(map[string][]*domain.GroupStanding)
	for k, st := range acc {
		st.GD = st.GF - st.GC
		grouped[k.group] = append(grouped[k.group], st)
	}

	// Sort each group: Pts DESC, GD DESC, GF DESC, Team ASC.
	for _, entries := range grouped {
		sort.Slice(entries, func(i, j int) bool {
			a, b := entries[i], entries[j]
			if a.Points != b.Points {
				return a.Points > b.Points
			}
			if a.GD != b.GD {
				return a.GD > b.GD
			}
			if a.GF != b.GF {
				return a.GF > b.GF
			}
			return a.Team < b.Team
		})
	}
	return grouped
}

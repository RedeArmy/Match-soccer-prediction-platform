package service

import "github.com/rede/world-cup-quiniela/pkg/footballprovider"

// DerivePenaltyWinner exposes the unexported helper for white-box testing.
var DerivePenaltyWinner = derivePenaltyWinner

// Fixture is re-exported so tests don't need to import the provider package directly.
type Fixture = footballprovider.Fixture

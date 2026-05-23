package domain

import "time"

// Country represents one of the three FIFA World Cup 2026 host nations.
// Code is the ISO 3166-1 alpha-2 identifier (e.g. "US", "MX", "CA").
type Country struct {
	ID   int
	Name string
	Code string
}

// State represents a US state, Mexican state, or Canadian province that hosts
// at least one FIFA World Cup 2026 venue. Code follows the standard postal
// abbreviation for the country (e.g. "NJ", "CDMX", "BC").
type State struct {
	ID        int
	Name      string
	Code      string
	CountryID int
	Country   *Country // hydrated by the repository when reading location data
}

// City is a host city for at least one FIFA World Cup 2026 venue.
type City struct {
	ID      int
	Name    string
	StateID int
	State   *State // hydrated by the repository when reading location data
}

// Stadium represents an official FIFA World Cup 2026 venue.
//
// This is reference data: the 16 host stadiums are fixed for the tournament
// and change only in exceptional circumstances (host-city withdrawal). Capacity
// is stored for display purposes; it is not used in any business rule.
//
// CityID is the foreign key to the cities table. City is the full location
// hierarchy (city -> state -> country) hydrated by the repository.
type Stadium struct {
	ID        int
	Name      string
	CityID    int
	City      *City // hydrated by the repository when reading location data
	Capacity  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

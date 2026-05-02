package handler

// CountryResponse is the JSON representation of a Country.
type CountryResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// StateResponse is the JSON representation of a State or province.
type StateResponse struct {
	ID      int             `json:"id"`
	Name    string          `json:"name"`
	Code    string          `json:"code"`
	Country CountryResponse `json:"country"`
}

// CityResponse is the JSON representation of a City.
type CityResponse struct {
	ID    int           `json:"id"`
	Name  string        `json:"name"`
	State StateResponse `json:"state"`
}

// StadiumResponse is the JSON representation of a Stadium with its full
// location hierarchy (city -> state -> country).
type StadiumResponse struct {
	ID       int          `json:"id"`
	Name     string       `json:"name"`
	City     CityResponse `json:"city"`
	Capacity int          `json:"capacity"`
}

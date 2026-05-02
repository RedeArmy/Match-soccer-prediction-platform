package handler

import "github.com/rede/world-cup-quiniela/internal/domain"

// TiebreakerConfigResponse is the JSON representation of the global tiebreaker
// configuration managed by the system administrator.
type TiebreakerConfigResponse struct {
	ID        int    `json:"id"`
	Question  string `json:"question"`
	Result    *int   `json:"result"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// TiebreakerResponse is the JSON representation of a single user tiebreaker prediction.
type TiebreakerResponse struct {
	ID         int    `json:"id"`
	UserID     int    `json:"user_id"`
	Prediction int    `json:"prediction"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// TiebreakerViewResponse is the JSON representation of GetMine: the group's
// question and the caller's own numeric prediction entry.
type TiebreakerViewResponse struct {
	Question *string             `json:"question"`
	Entry    *TiebreakerResponse `json:"entry"`
}

func tiebreakerConfigToResponse(cfg *domain.TiebreakerConfig) TiebreakerConfigResponse {
	return TiebreakerConfigResponse{
		ID:        cfg.ID,
		Question:  cfg.Question,
		Result:    cfg.Result,
		CreatedAt: cfg.CreatedAt.Format(timeFormat),
		UpdatedAt: cfg.UpdatedAt.Format(timeFormat),
	}
}

func tiebreakerToResponse(tb *domain.Tiebreaker) TiebreakerResponse {
	return TiebreakerResponse{
		ID:         tb.ID,
		UserID:     tb.UserID,
		Prediction: tb.Prediction,
		CreatedAt:  tb.CreatedAt.Format(timeFormat),
		UpdatedAt:  tb.UpdatedAt.Format(timeFormat),
	}
}

func tiebreakerViewToResponse(view *domain.TiebreakerView) TiebreakerViewResponse {
	resp := TiebreakerViewResponse{Question: view.Question}
	if view.Entry != nil {
		r := tiebreakerToResponse(view.Entry)
		resp.Entry = &r
	}
	return resp
}

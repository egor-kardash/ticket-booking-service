package http

import (
	"github.com/egor-kardash/ticket-booking-service/internal/features/auth/domain"
)

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type userResponse struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

func toUserResponse(user domain.User) userResponse {
	return userResponse{
		ID:    user.ID,
		Email: user.Email,
	}
}

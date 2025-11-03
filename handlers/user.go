package handlers

import (
	"net/http"

	"backend/middleware"
)

func GetOrCreateUserID(r *http.Request) (int, bool) {
	userID, ok := middleware.GetUserIDFromContext(r)
	return userID, ok
}

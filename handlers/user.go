package handlers

import (
	"net/http"

	"backend/middleware"
)

// GetOrCreateUserID gets the database user ID from the session
func GetOrCreateUserID(r *http.Request) (int, bool) {
	userID, ok := middleware.GetUserIDFromContext(r)
	return userID, ok
}

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

// User model.
type User struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatarUrl"`
}

// GET /api/usernames?search={search}
func searchUsernames(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if search == "" {
		respond(w, Errors{map[string]string{
			"search": "Search required",
		}}, http.StatusUnprocessableEntity)
		return
	}

	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)

	rows, err := db.QueryContext(ctx, `
		SELECT username
		FROM users
		WHERE id != $1
			AND username ILIKE $2 || '%'
		ORDER BY username
		LIMIT 5
	`, authUserID, search)
	if err != nil {
		respondError(w, fmt.Errorf("could not query usernames: %v", err))
		return
	}
	defer rows.Close()

	usernames := make([]string, 0)
	for rows.Next() {
		var username string
		if err = rows.Scan(&username); err != nil {
			respondError(w, fmt.Errorf("could not scan username: %v", err))
			return
		}

		usernames = append(usernames, username)
	}

	if err = rows.Err(); err != nil {
		respondError(w, fmt.Errorf("could not iterate over usernames: %v", err))
		return
	}

	respond(w, usernames, http.StatusOK)
}

func queryUser(ctx context.Context, rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}, id string) (User, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var user User
	if err := rowQuerier.QueryRowContext(ctx, `
		SELECT username, avatar_url FROM users WHERE id = $1
	`, id).Scan(&user.Username, &user.AvatarURL); err != nil {
		return user, err
	}

	user.ID = id
	return user, nil
}

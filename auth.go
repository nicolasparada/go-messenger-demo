package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/knq/jwt"
	"github.com/matoous/go-nanoid"
)

const jwtLifetime = time.Hour * 24 * 14 // 14 days.

// GithubUser data.
type GithubUser struct {
	ID        int     `json:"id"`
	Login     string  `json:"login"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

// POST /api/login
func login(w http.ResponseWriter, r *http.Request) {
	if !inLocalhost {
		http.NotFound(w, r)
		return
	}

	var input struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var user User
	if err := db.QueryRowContext(r.Context(), `
		SELECT id, avatar_url
		FROM users
		WHERE username = $1
	`, input.Username).Scan(
		&user.ID,
		&user.AvatarURL,
	); err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query user: %v", err))
		return
	}

	user.Username = input.Username

	exp := time.Now().Add(jwtLifetime)
	token, err := issueToken(user.ID, exp)
	if err != nil {
		respondError(w, fmt.Errorf("could not create token: %v", err))
		return
	}

	respond(w, map[string]interface{}{
		"authUser":  user,
		"token":     token,
		"expiresAt": exp,
	}, http.StatusOK)
}

// GET /api/oauth/github
func githubOAuthStart(w http.ResponseWriter, r *http.Request) {
	state, err := gonanoid.Nanoid()
	if err != nil {
		respondError(w, fmt.Errorf("could not generte state: %v", err))
		return
	}

	stateCookieValue, err := cookieSigner.Encode("state", state)
	if err != nil {
		respondError(w, fmt.Errorf("could not encode state cookie: %v", err))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "state",
		Value:    stateCookieValue,
		Path:     "/api/oauth/github",
		HttpOnly: true,
		Secure:   !inLocalhost,
		// SameSite: http.SameSiteLaxMode, // TODO: enable SameSite in state cookie for go 1.11.
	})
	http.Redirect(w, r, githubOAuthConfig.AuthCodeURL(state), http.StatusTemporaryRedirect)
}

// GET /api/oauth/github/callback
func githubOAuthCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("state")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusTeapot), http.StatusTeapot)
		return
	}

	defer http.SetCookie(w, &http.Cookie{Name: "state", Value: "", MaxAge: -1})

	var state string
	if err = cookieSigner.Decode("state", stateCookie.Value, &state); err != nil {
		http.Error(w, http.StatusText(http.StatusTeapot), http.StatusTeapot)
		return
	}

	q := r.URL.Query()

	if state != q.Get("state") {
		http.Error(w, http.StatusText(http.StatusTeapot), http.StatusTeapot)
		return
	}

	ctx := r.Context()

	t, err := githubOAuthConfig.Exchange(ctx, q.Get("code"))
	if err != nil {
		respondError(w, fmt.Errorf("could not fetch github token: %v", err))
		return
	}

	client := githubOAuthConfig.Client(ctx, t)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		respondError(w, fmt.Errorf("could not fetch github user: %v", err))
		return
	}

	var githubUser GithubUser
	if err = json.NewDecoder(resp.Body).Decode(&githubUser); err != nil {
		respondError(w, fmt.Errorf("could not decode github user: %v", err))
		return
	}
	defer resp.Body.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, fmt.Errorf("could not begin tx: %v", err))
		return
	}

	var user User
	if err = tx.QueryRow(`
		SELECT id, username, avatar_url FROM users WHERE github_id = $1
	`, githubUser.ID).Scan(&user.ID, &user.Username, &user.AvatarURL); err == sql.ErrNoRows {
		if err = tx.QueryRow(`
			INSERT INTO users (username, avatar_url, github_id) VALUES ($1, $2, $3)
			RETURNING id
		`, githubUser.Login, githubUser.AvatarURL, githubUser.ID).Scan(&user.ID); err != nil {
			respondError(w, fmt.Errorf("could not insert user: %v", err))
			return
		}
		user.Username = githubUser.Login
		user.AvatarURL = githubUser.AvatarURL
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query user by github ID: %v", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit to finish github oauth: %v", err))
		return
	}

	exp := time.Now().Add(jwtLifetime)
	token, err := issueToken(user.ID, exp)
	if err != nil {
		respondError(w, fmt.Errorf("could not issue token: %v", err))
		return
	}

	expiresAt, _ := exp.MarshalText()

	data := make(url.Values)
	data.Set("token", token)
	data.Set("expires_at", string(expiresAt))

	callbackURL := *origin
	callbackURL.Path = "/callback"
	callbackURL.RawQuery = data.Encode()

	http.Redirect(w, r, callbackURL.String(), http.StatusTemporaryRedirect)
}

// GET /api/auth_user
func getAuthUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)

	user, err := queryUser(ctx, db, authUserID)
	if err == sql.ErrNoRows {
		http.Error(w, http.StatusText(http.StatusTeapot), http.StatusTeapot)
		return
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query auth user: %v", err))
		return
	}

	respond(w, user, http.StatusOK)
}

// POST /api/refresh_token
func refreshToken(w http.ResponseWriter, r *http.Request) {
	authUserID := r.Context().Value(keyAuthUserID).(string)

	exp := time.Now().Add(jwtLifetime)
	token, err := issueToken(authUserID, exp)
	if err != nil {
		respondError(w, fmt.Errorf("could not issue token: %v", err))
		return
	}

	respond(w, map[string]interface{}{
		"token":     token,
		"expiresAt": exp,
	}, http.StatusOK)
}

func guard(handler http.HandlerFunc) http.HandlerFunc {
	guarded := func(w http.ResponseWriter, r *http.Request) {
		var token string
		if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
			token = a[7:]
		} else if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
			token = t
		} else {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		var claims jwt.Claims
		if err := jwtSigner.Decode([]byte(token), &claims); err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, keyAuthUserID, claims.Subject)

		handler(w, r.WithContext(ctx))
	}
	return guarded
}

func issueToken(subject string, exp time.Time) (string, error) {
	token, err := jwtSigner.Encode(jwt.Claims{
		Subject:    subject,
		Expiration: json.Number(strconv.FormatInt(exp.Unix(), 10)),
	})
	if err != nil {
		return "", err
	}
	return string(token), nil
}

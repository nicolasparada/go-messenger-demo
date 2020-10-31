package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/matryer/way"
)

// Conversation model.
type Conversation struct {
	ID                string   `json:"id"`
	OtherParticipant  *User    `json:"otherParticipant"`
	LastMessage       *Message `json:"lastMessage"`
	HasUnreadMessages bool     `json:"hasUnreadMessages"`
}

// POST /api/conversations
func createConversation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" {
		respond(w, Errors{map[string]string{
			"username": "Username required",
		}}, http.StatusUnprocessableEntity)
		return
	}

	ctx := r.Context()
	uid := ctx.Value(keyAuthUserID).(string)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, fmt.Errorf("could not begin tx: %w", err))
		return
	}

	defer func() { _ = tx.Rollback() }()

	var otherParticipant User
	if err := tx.QueryRow(`
		SELECT id, avatar_url FROM users WHERE username = $1
	`, in.Username).Scan(
		&otherParticipant.ID,
		&otherParticipant.AvatarURL,
	); err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query other participant: %w", err))
		return
	}

	otherParticipant.Username = in.Username

	if otherParticipant.ID == uid {
		http.Error(w, "Try start a conversation with someone else", http.StatusForbidden)
		return
	}

	var cid string
	if err := tx.QueryRow(`
		SELECT conversation_id FROM participants WHERE user_id = $1
		INTERSECT
		SELECT conversation_id FROM participants WHERE user_id = $2
	`, uid, otherParticipant.ID).Scan(&cid); err != nil && err != sql.ErrNoRows {
		respondError(w, fmt.Errorf("could not query common conversation id: %w", err))
		return
	} else if err == nil {
		http.Redirect(w, r, "/api/conversations/"+cid, http.StatusFound)
		return
	}

	var c Conversation
	if err = tx.QueryRow(`
		INSERT INTO conversations DEFAULT VALUES
		RETURNING id
	`).Scan(&c.ID); err != nil {
		respondError(w, fmt.Errorf("could not insert conversation: %w", err))
		return
	}

	if _, err = tx.Exec(`
		INSERT INTO participants (user_id, conversation_id) VALUES
			($1, $2),
			($3, $2)
	`, uid, c.ID, otherParticipant.ID); err != nil {
		respondError(w, fmt.Errorf("could not insert participants: %w", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit tx to create conversation: %w", err))
		return
	}

	c.OtherParticipant = &otherParticipant

	respond(w, c, http.StatusCreated)
}

// GET /api/conversations?before={before}
func getConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := ctx.Value(keyAuthUserID).(string)

	query := `
		SELECT
			conversations.id,
			auth_user.messages_read_at < messages.created_at AS has_unread_messages,
			messages.id,
			messages.content,
			messages.created_at,
			messages.user_id = $1 AS mine,
			other_users.id,
			other_users.username,
			other_users.avatar_url
		FROM conversations
		INNER JOIN messages ON conversations.last_message_id = messages.id
		INNER JOIN participants other_participants
			ON other_participants.conversation_id = conversations.id
				AND other_participants.user_id != $1
		INNER JOIN users other_users ON other_participants.user_id = other_users.id
		INNER JOIN participants auth_user
			ON auth_user.conversation_id = conversations.id
				AND auth_user.user_id = $1`
	args := []interface{}{uid}

	if before := strings.TrimSpace(r.URL.Query().Get("before")); before != "" {
		query += " WHERE conversations.id > $2"
		args = append(args, before)
	}

	query += `
		ORDER BY messages.created_at DESC
		LIMIT 25`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		respondError(w, fmt.Errorf("could not query conversations: %w", err))
		return
	}
	defer rows.Close()

	cc := make([]Conversation, 0, 25)
	for rows.Next() {
		var c Conversation
		var m Message
		var u User
		if err = rows.Scan(
			&c.ID,
			&c.HasUnreadMessages,
			&m.ID,
			&m.Content,
			&m.CreatedAt,
			&m.Mine,
			&u.ID,
			&u.Username,
			&u.AvatarURL,
		); err != nil {
			respondError(w, fmt.Errorf("could not scan conversation: %w", err))
			return
		}

		c.LastMessage = &m
		c.OtherParticipant = &u
		cc = append(cc, c)
	}

	if err = rows.Err(); err != nil {
		respondError(w, fmt.Errorf("could not iterate over conversations: %w", err))
		return
	}

	respond(w, cc, http.StatusOK)
}

// GET /api/conversations/{conversation_id}
func getConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := ctx.Value(keyAuthUserID).(string)
	cid := way.Param(ctx, "conversation_id")

	var c Conversation
	var u User
	if err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(auth_user.messages_read_at < messages.created_at, false) AS has_unread_messages,
			other_users.id,
			other_users.username,
			other_users.avatar_url
		FROM conversations
		LEFT JOIN messages ON conversations.last_message_id = messages.id
		INNER JOIN participants other_participants
			ON other_participants.conversation_id = conversations.id
				AND other_participants.user_id != $1
		INNER JOIN users other_users ON other_participants.user_id = other_users.id
		INNER JOIN participants auth_user
			ON auth_user.conversation_id = conversations.id
				AND auth_user.user_id = $1
		WHERE conversations.id = $2
	`, uid, cid).Scan(
		&c.HasUnreadMessages,
		&u.ID,
		&u.Username,
		&u.AvatarURL,
	); err == sql.ErrNoRows {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query conversation: %w", err))
		return
	}

	c.ID = cid
	c.OtherParticipant = &u

	respond(w, c, http.StatusOK)
}

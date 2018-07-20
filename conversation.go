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
	var input struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" {
		respond(w, Errors{map[string]string{
			"username": "Username required",
		}}, http.StatusUnprocessableEntity)
		return
	}

	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, fmt.Errorf("could not begin tx: %v", err))
		return
	}
	defer tx.Rollback()

	var otherParticipant User
	if err := tx.QueryRow(`
		SELECT id, avatar_url FROM users WHERE username = $1
	`, input.Username).Scan(
		&otherParticipant.ID,
		&otherParticipant.AvatarURL,
	); err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query other participant: %v", err))
		return
	}

	otherParticipant.Username = input.Username

	if otherParticipant.ID == authUserID {
		http.Error(w, "Try start a conversation with someone else", http.StatusForbidden)
		return
	}

	var conversationID string
	if err := tx.QueryRow(`
		SELECT conversation_id FROM participants WHERE user_id = $1
		INTERSECT
		SELECT conversation_id FROM participants WHERE user_id = $2
	`, authUserID, otherParticipant.ID).Scan(&conversationID); err != nil && err != sql.ErrNoRows {
		respondError(w, fmt.Errorf("could not query common conversation id: %v", err))
		return
	} else if err == nil {
		http.Redirect(w, r, "/api/conversations/"+conversationID, http.StatusFound)
		return
	}

	var conversation Conversation
	if err = tx.QueryRow(`
		INSERT INTO conversations DEFAULT VALUES
		RETURNING id
	`).Scan(&conversation.ID); err != nil {
		respondError(w, fmt.Errorf("could not insert conversation: %v", err))
		return
	}

	if _, err = tx.Exec(`
		INSERT INTO participants (user_id, conversation_id) VALUES
			($1, $2),
			($3, $2)
	`, authUserID, conversation.ID, otherParticipant.ID); err != nil {
		respondError(w, fmt.Errorf("could not insert participants: %v", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit tx to create conversation: %v", err))
		return
	}

	conversation.OtherParticipant = &otherParticipant

	respond(w, conversation, http.StatusCreated)
}

// GET /api/conversations?before={before}
func getConversations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)

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
	args := []interface{}{authUserID}

	if before := strings.TrimSpace(r.URL.Query().Get("before")); before != "" {
		query += " WHERE conversations.id > $2"
		args = append(args, before)
	}

	query += `
		ORDER BY messages.created_at DESC
		LIMIT 25`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		respondError(w, fmt.Errorf("could not query conversations: %v", err))
		return
	}
	defer rows.Close()

	conversations := make([]Conversation, 0)
	for rows.Next() {
		var conversation Conversation
		var lastMessage Message
		var otherParticipant User
		if err = rows.Scan(
			&conversation.ID,
			&conversation.HasUnreadMessages,
			&lastMessage.ID,
			&lastMessage.Content,
			&lastMessage.CreatedAt,
			&lastMessage.Mine,
			&otherParticipant.ID,
			&otherParticipant.Username,
			&otherParticipant.AvatarURL,
		); err != nil {
			respondError(w, fmt.Errorf("could not scan conversation: %v", err))
			return
		}

		conversation.LastMessage = &lastMessage
		conversation.OtherParticipant = &otherParticipant
		conversations = append(conversations, conversation)
	}

	if err = rows.Err(); err != nil {
		respondError(w, fmt.Errorf("could not iterate over conversations: %v", err))
		return
	}

	respond(w, conversations, http.StatusOK)
}

// GET /api/conversations/{conversation_id}
func getConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)
	conversationID := way.Param(ctx, "conversation_id")

	var conversation Conversation
	var otherParticipant User
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
	`, authUserID, conversationID).Scan(
		&conversation.HasUnreadMessages,
		&otherParticipant.ID,
		&otherParticipant.Username,
		&otherParticipant.AvatarURL,
	); err == sql.ErrNoRows {
		http.Error(w, "Conversation not found", http.StatusNotFound)
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query conversation: %v", err))
		return
	}

	conversation.ID = conversationID
	conversation.OtherParticipant = &otherParticipant

	respond(w, conversation, http.StatusOK)
}

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/matryer/way"
)

var rxSpaces = regexp.MustCompile("\\s+")

// Message model.
type Message struct {
	ID             string    `json:"id"`
	Content        string    `json:"content"`
	UserID         string    `json:"-"`
	ConversationID string    `json:"conversationId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	Mine           bool      `json:"mine"`
	ReceiverID     string    `json:"-"`
}

// POST /api/conversations/{conversation_id}/messages
func createMessage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	errs := make(map[string]string)
	input.Content = removeSpaces(input.Content)
	if input.Content == "" {
		errs["content"] = "Message content required"
	} else if len([]rune(input.Content)) > 480 {
		errs["content"] = "Message too long. 480 max"
	}
	if len(errs) != 0 {
		respond(w, Errors{errs}, http.StatusUnprocessableEntity)
		return
	}

	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)
	conversationID := way.Param(ctx, "conversation_id")

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, fmt.Errorf("could not begin tx: %v", err))
		return
	}
	defer tx.Rollback()

	isParticipant, err := queryParticipantExistance(ctx, tx, authUserID, conversationID)
	if err != nil {
		respondError(w, fmt.Errorf("could not query participant existance: %v", err))
		return
	}

	if !isParticipant {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	var message Message
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO messages (content, user_id, conversation_id) VALUES
			($1, $2, $3)
		RETURNING id, created_at
	`, input.Content, authUserID, conversationID).Scan(
		&message.ID,
		&message.CreatedAt,
	); err != nil {
		respondError(w, fmt.Errorf("could not insert message: %v", err))
		return
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations SET last_message_id = $1
		WHERE id = $2
	`, message.ID, conversationID); err != nil {
		respondError(w, fmt.Errorf("could not update conversation last message ID: %v", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit tx to create a message: %v", err))
		return
	}

	go func() {
		if err = updateMessagesReadAt(nil, authUserID, conversationID); err != nil {
			log.Printf("could not update messages read at: %v\n", err)
		}
	}()

	message.Content = input.Content
	message.UserID = authUserID
	message.ConversationID = conversationID
	go newMessageCreated(message)
	message.Mine = true

	respond(w, message, http.StatusCreated)
}

func removeSpaces(s string) string {
	if s == "" {
		return s
	}

	lines := make([]string, 0)
	for _, line := range strings.Split(s, "\n") {
		line = rxSpaces.ReplaceAllLiteralString(line, " ")
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func newMessageCreated(message Message) error {
	if err := db.QueryRow(`
		SELECT user_id FROM participants
		WHERE user_id != $1 and conversation_id = $2
	`, message.UserID, message.ConversationID).Scan(&message.ReceiverID); err != nil {
		return err
	}

	messageBus.send(message)
	return nil
}

// GET /api/conversations/{conversation_id}/messages?before={before}
func getMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)
	conversationID := way.Param(ctx, "conversation_id")

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		respondError(w, fmt.Errorf("could not begin tx: %v", err))
		return
	}
	defer tx.Rollback()

	isParticipant, err := queryParticipantExistance(ctx, tx, authUserID, conversationID)
	if err != nil {
		respondError(w, fmt.Errorf("could not query participant existance: %v", err))
		return
	}

	if !isParticipant {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	query := `
		SELECT
			id,
			content,
			created_at,
			user_id = $1 AS mine
		FROM messages
		WHERE conversation_id = $2`
	args := []interface{}{authUserID, conversationID}

	if before := strings.TrimSpace(r.URL.Query().Get("before")); before != "" {
		query += ` AND id < $3`
		args = append(args, before)
	}

	query += `
		ORDER BY created_at DESC
		LIMIT 25`

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		respondError(w, fmt.Errorf("could not query messages: %v", err))
		return
	}
	defer rows.Close()

	messages := make([]Message, 0)
	for rows.Next() {
		var message Message
		if err = rows.Scan(
			&message.ID,
			&message.Content,
			&message.CreatedAt,
			&message.Mine,
		); err != nil {
			respondError(w, fmt.Errorf("could not scan message: %v", err))
			return
		}

		messages = append(messages, message)
	}

	if err = rows.Err(); err != nil {
		respondError(w, fmt.Errorf("could not iterate over messages: %v", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit tx to get messages: %v", err))
		return
	}

	go func() {
		if err = updateMessagesReadAt(nil, authUserID, conversationID); err != nil {
			log.Printf("could not update messages read at: %v\n", err)
		}
	}()

	respond(w, messages, http.StatusOK)
}

// GET /api/messages
func subscribeToMessages(w http.ResponseWriter, r *http.Request) {
	if a := r.Header.Get("Accept"); !strings.Contains(a, "text/event-stream") {
		http.Error(w, "This endpoint requires an EventSource connection", http.StatusNotAcceptable)
		return
	}

	f, ok := w.(http.Flusher)
	if !ok {
		respondError(w, errors.New("streaming unsupported"))
		return
	}

	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)

	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("Content-Type", "text/event-stream")

	ch := make(chan Message)
	defer close(ch)
	defer messageBus.register(ch, authUserID)()

	for {
		select {
		case <-w.(http.CloseNotifier).CloseNotify():
			return
		case <-time.After(time.Second * 15):
			fmt.Fprint(w, "ping: \n\n")
			f.Flush()
		case message := <-ch:
			if b, err := json.Marshal(message); err != nil {
				log.Printf("could not marshall message: %v\n", err)
				fmt.Fprintf(w, "error: %v\n\n", err)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", b)
			}
			f.Flush()
		}
	}
}

func readMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authUserID := ctx.Value(keyAuthUserID).(string)
	conversationID := way.Param(ctx, "conversation_id")

	if err := updateMessagesReadAt(ctx, authUserID, conversationID); err != nil {
		respondError(w, fmt.Errorf("could not update messages read at: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func updateMessagesReadAt(ctx context.Context, userID, conversationID string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if _, err := db.ExecContext(ctx, `
		UPDATE participants SET messages_read_at = now()
		WHERE user_id = $1 AND conversation_id = $2
	`, userID, conversationID); err != nil {
		return err
	}
	return nil
}

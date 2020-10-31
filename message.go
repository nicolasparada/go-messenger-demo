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

// MessageClient to subscribe to new messages.
type MessageClient struct {
	Messages chan Message
	UserID   string
}

// POST /api/conversations/{conversation_id}/messages
func createMessage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	errs := make(map[string]string)
	in.Content = removeSpaces(in.Content)
	if in.Content == "" {
		errs["content"] = "Message content required"
	} else if len([]rune(in.Content)) > 480 {
		errs["content"] = "Message too long. 480 max"
	}
	if len(errs) != 0 {
		respond(w, Errors{errs}, http.StatusUnprocessableEntity)
		return
	}

	ctx := r.Context()
	uid := ctx.Value(keyAuthUserID).(string)
	cid := way.Param(ctx, "conversation_id")

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, fmt.Errorf("could not begin tx: %w", err))
		return
	}
	defer tx.Rollback()

	isParticipant, err := queryParticipantExistance(ctx, tx, uid, cid)
	if err != nil {
		respondError(w, fmt.Errorf("could not query participant existance: %w", err))
		return
	}

	if !isParticipant {
		http.Error(w, "Conversation not found", http.StatusNotFound)
		return
	}

	var m Message
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO messages (content, user_id, conversation_id) VALUES
			($1, $2, $3)
		RETURNING id, created_at
	`, in.Content, uid, cid).Scan(
		&m.ID,
		&m.CreatedAt,
	); err != nil {
		respondError(w, fmt.Errorf("could not insert message: %w", err))
		return
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations SET last_message_id = $1
		WHERE id = $2
	`, m.ID, cid); err != nil {
		respondError(w, fmt.Errorf("could not update conversation last message ID: %w", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit tx to create a message: %w", err))
		return
	}

	go func() {
		if err = updateMessagesReadAt(nil, uid, cid); err != nil {
			log.Printf("could not update messages read at: %v\n", err)
		}
	}()

	m.Content = in.Content
	m.UserID = uid
	m.ConversationID = cid

	go messageCreated(m)

	m.Mine = true

	respond(w, m, http.StatusCreated)
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

func messageCreated(m Message) error {
	if err := db.QueryRow(`
		SELECT user_id FROM participants
		WHERE user_id != $1 and conversation_id = $2
	`, m.UserID, m.ConversationID).Scan(&m.ReceiverID); err != nil {
		return err
	}

	go broadcastMessage(m)

	return nil
}

// GET /api/conversations/{conversation_id}/messages?before={before}
func getMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := ctx.Value(keyAuthUserID).(string)
	cid := way.Param(ctx, "conversation_id")

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		respondError(w, fmt.Errorf("could not begin tx: %w", err))
		return
	}
	defer tx.Rollback()

	isParticipant, err := queryParticipantExistance(ctx, tx, uid, cid)
	if err != nil {
		respondError(w, fmt.Errorf("could not query participant existance: %w", err))
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
	args := []interface{}{uid, cid}

	if before := strings.TrimSpace(r.URL.Query().Get("before")); before != "" {
		query += ` AND id < $3`
		args = append(args, before)
	}

	query += `
		ORDER BY created_at DESC
		LIMIT 25`

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		respondError(w, fmt.Errorf("could not query messages: %w", err))
		return
	}
	defer rows.Close()

	mm := make([]Message, 0, 25)
	for rows.Next() {
		var message Message
		if err = rows.Scan(
			&message.ID,
			&message.Content,
			&message.CreatedAt,
			&message.Mine,
		); err != nil {
			respondError(w, fmt.Errorf("could not scan message: %w", err))
			return
		}

		mm = append(mm, message)
	}

	if err = rows.Err(); err != nil {
		respondError(w, fmt.Errorf("could not iterate over messages: %w", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit tx to get messages: %w", err))
		return
	}

	go func() {
		if err = updateMessagesReadAt(nil, uid, cid); err != nil {
			log.Printf("could not update messages read at: %v\n", err)
		}
	}()

	respond(w, mm, http.StatusOK)
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
	uid := ctx.Value(keyAuthUserID).(string)

	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("Content-Type", "text/event-stream")

	mm := make(chan Message)
	defer close(mm)

	client := &MessageClient{Messages: mm, UserID: uid}
	messageClients.Store(client, nil)
	defer messageClients.Delete(client)

	for {
		select {
		case <-ctx.Done():
			return
		case m := <-mm:
			if b, err := json.Marshal(m); err != nil {
				log.Printf("could not marshall message: %v\n", err)
				fmt.Fprintf(w, "event: error\ndata: %v\n\n", err)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", b)
			}
			f.Flush()
		}
	}
}

func readMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := ctx.Value(keyAuthUserID).(string)
	cid := way.Param(ctx, "conversation_id")

	if err := updateMessagesReadAt(ctx, uid, cid); err != nil {
		respondError(w, fmt.Errorf("could not update messages read at: %w", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func updateMessagesReadAt(ctx context.Context, userID, cid string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if _, err := db.ExecContext(ctx, `
		UPDATE participants SET messages_read_at = now()
		WHERE user_id = $1 AND conversation_id = $2
	`, userID, cid); err != nil {
		return err
	}
	return nil
}

func broadcastMessage(m Message) {
	messageClients.Range(func(key, _ interface{}) bool {
		client := key.(*MessageClient)
		if client.UserID == m.ReceiverID {
			client.Messages <- m
		}
		return true
	})
}

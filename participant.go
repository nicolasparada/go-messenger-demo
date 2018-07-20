package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/matryer/way"
)

// GET /api/conversations/{conversation_id}/other_participant
func getOtherParticipantFromConversation(w http.ResponseWriter, r *http.Request) {
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

	var otherUser User
	if err = tx.QueryRowContext(ctx, `
		SELECT
			users.id,
			users.username,
			users.avatar_url
		FROM participants
		INNER JOIN users ON participants.user_id = users.id
		WHERE participants.user_id != $1
			participants.conversation_id = $2
		LIMIT 1
	`, authUserID, conversationID).Scan(
		&otherUser.ID,
		&otherUser.Username,
		&otherUser.AvatarURL,
	); err == sql.ErrNoRows {
		http.Error(w, "Could not find the other participant of this conversation", http.StatusNotFound)
		return
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query other participant from conversation: %v", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit tx to get other participant from conversation: %v", err))
		return
	}

	respond(w, otherUser, http.StatusOK)
}

func queryParticipantExistance(ctx context.Context, tx *sql.Tx, userID, conversationID string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM participants
		WHERE user_id = $1 AND conversation_id = $2
	)`, userID, conversationID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"github.com/matryer/way"
)

// GET /api/conversations/{conversation_id}/other_participant
func getOtherParticipantFromConversation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := ctx.Value(keyAuthUserID).(string)
	cid := way.Param(ctx, "conversation_id")

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		respondError(w, fmt.Errorf("could not begin tx: %w", err))
		return
	}

	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("failed to rollback other participants from conversation retrieval: %v\n", err)
		}
	}()

	isParticipant, err := queryParticipantExistance(ctx, tx, uid, cid)
	if err != nil {
		respondError(w, fmt.Errorf("could not query participant existance: %w", err))
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
			AND participants.conversation_id = $2
		LIMIT 1
	`, uid, cid).Scan(
		&otherUser.ID,
		&otherUser.Username,
		&otherUser.AvatarURL,
	); err == sql.ErrNoRows {
		http.Error(w, "Could not find the other participant of this conversation", http.StatusNotFound)
		return
	} else if err != nil {
		respondError(w, fmt.Errorf("could not query other participant from conversation: %w", err))
		return
	}

	if err = tx.Commit(); err != nil {
		respondError(w, fmt.Errorf("could not commit tx to get other participant from conversation: %w", err))
		return
	}

	respond(w, otherUser, http.StatusOK)
}

func queryParticipantExistance(ctx context.Context, tx *sql.Tx, userID, cid string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM participants
		WHERE user_id = $1 AND conversation_id = $2
	)`, userID, cid).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

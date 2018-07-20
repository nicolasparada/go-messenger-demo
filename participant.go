package main

import (
	"context"
	"database/sql"
)

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

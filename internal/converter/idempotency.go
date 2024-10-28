package converter

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"
)

func IsProcessed(db *sql.DB, videoID int) bool {
	var is_processed bool
	query := "SELECT EXISTS (SELECT 1 FROM processed_videos WHERE video_id = $1 and status = 'success')"
	err := db.QueryRow(query, videoID).Scan(&is_processed)
	if err != nil {
		slog.Error("Error checking if video is processed", slog.Int("video_id", videoID), err)
		return false
	}
	return is_processed
}

func MarkAsProcessed(db *sql.DB, videoID int) error {
	query := "INSERT INTO processed_videos (video_id, status, processed_at) VALUES ($1, $2, $3)"
	_, err := db.Exec(query, videoID, "success", time.Now())
	if err != nil {
		slog.Error("Error marking video as processed", slog.Int("video_id", videoID), err)
		return err
	}
	return nil
}

func RegisterError(db *sql.DB, errorData map[string]any, err error) {
	serializedError, _ := json.Marshal(errorData)
	query := "INSERT INTO process_errors_log (error_details, created_at) VALUES ($1, $2)"
	_, dbErr := db.Exec(query, serializedError, time.Now())
	if dbErr != nil {
		slog.Error("Error registering error", slog.String("video_id", string(serializedError)))
	}
}

package converter

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
	"wd-tube/internal/rabbitmq"

	"github.com/streadway/amqp"
)

type VideoConverter struct {
	db             *sql.DB
	rabbitmqClient *rabbitmq.RabbitClient
}

func NewVideoConverter(db *sql.DB, rabbitmqClient rabbitmq.RabbitClient) *VideoConverter {
	return &VideoConverter{
		db:             db,
		rabbitmqClient: &rabbitmqClient,
	}
}

type VideoTask struct {
	VideoID int    `json:"video_id"`
	Path    string `json:"path"`
}

func (vc *VideoConverter) Handle(d amqp.Delivery, conversionExch, confirmationQueue, confirmationKey string) {
	var task VideoTask
	err := json.Unmarshal(d.Body, &task)
	if err != nil {
		vc.logError(task, "failed to unmarshal task", err)
		return
	}

	if IsProcessed(vc.db, task.VideoID) {
		slog.Warn("Video already processed", slog.Int("video_id", task.VideoID))
		d.Ack(false)
		return
	}

	err = vc.procecssVideo(&task)
	if err != nil {
		vc.logError(task, "failed to process video", err)
		return
	}
	err = MarkAsProcessed(vc.db, task.VideoID)
	if err != nil {
		vc.logError(task, "failed to mark video as processed", err)
		return
	}
	d.Ack(false)
	slog.Info("\033[32m"+"Video processing completed"+"\033[0m", slog.Int("video_id", task.VideoID))

	confirmationMessage := []byte(fmt.Sprintf(`{"video_id": %d, "path":"%s"}`, task.VideoID, task.Path))

	err = vc.rabbitmqClient.PublishMessage(conversionExch, confirmationKey, confirmationQueue, confirmationMessage)

	if err != nil {
		vc.logError(task, "failed to publish confirmation message", err)
	}
}

func (vc *VideoConverter) procecssVideo(task *VideoTask) error {
	mergedFile := filepath.Join(task.Path, "merged.mp4")
	mpegDashPath := filepath.Join(task.Path, "mpeg-dash")

	slog.Info("Merging chunks", slog.String("path", task.Path))
	err := vc.mergeChunks(task.Path, mergedFile)
	if err != nil {
		vc.logError(*task, "failed to merge chunks", err)
		return err
	}
	slog.Info("Creating mpeg-dash", slog.String("path", task.Path))
	err = os.MkdirAll(mpegDashPath, os.ModePerm)
	if err != nil {
		vc.logError(*task, "failed to create mpeg-dash directory", err)
		return err
	}
	slog.Info("Converting video to mpeg-dash", slog.String("path", task.Path))
	ffmpegCmd := exec.Command(
		"ffmpeg", "-i", mergedFile,
		"-f", "dash",
		filepath.Join(mpegDashPath, "output.mpd"),
	)
	output, err := ffmpegCmd.CombinedOutput()
	if err != nil {
		vc.logError(*task, "failed to convert video to mpeg-dash "+string(output), err)
		return err
	}
	slog.Info("Video processing completed", slog.String("path", task.Path))

	err = os.Remove(mergedFile)
	if err != nil {
		vc.logError(*task, "failed to remove merged file", err)
		return err
	}
	return nil
}

func (vc *VideoConverter) logError(task VideoTask, message string, err error) {
	errorData := map[string]interface{}{
		"video_id": task.VideoID,
		"error":    message,
		"details":  err.Error(),
		"time":     time.Now(),
	}
	serialized, _ := json.Marshal(errorData)
	slog.Error("\033[31m"+"Processing error"+"\033[0m", slog.String("error_details", string(serialized)))

	RegisterError(vc.db, errorData, err)
}

func (vc *VideoConverter) extractNumber(filename string) int {
	re := regexp.MustCompile(`\d+`)
	numStr := re.FindString(filepath.Base(filename))
	num, error := strconv.Atoi(numStr)
	if error != nil {
		return -1
	}
	return num
}

func (vc *VideoConverter) mergeChunks(inputDir, outputFile string) error {
	chunks, err := filepath.Glob(filepath.Join(inputDir, "*.chunk"))
	if err != nil {
		return fmt.Errorf("failed to list chunks: %v", err)
	}

	sort.Slice(chunks, func(i, j int) bool {
		return vc.extractNumber(chunks[i]) < vc.extractNumber(chunks[j])
	})

	output, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer output.Close()

	for _, chunk := range chunks {
		input, err := os.Open(chunk)
		if err != nil {
			return fmt.Errorf("failed to open chunk %s: %v", chunk, err)
		}
		defer input.Close()

		_, err = output.ReadFrom(input)
		if err != nil {
			return fmt.Errorf("failed to write chunk %s to output: %v", chunk, err)
		}
	}
	return nil
}

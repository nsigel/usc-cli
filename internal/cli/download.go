package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/nsigel/usc-cli/brightspace"
	"github.com/spf13/cobra"
)

func downloadCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "download COURSE_ID TOPIC_ID",
		Short: "Download a Brightspace content item",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			courseID, err := positiveID(args[0], "COURSE_ID")
			if err != nil {
				return err
			}
			topicID, err := positiveID(args[1], "TOPIC_ID")
			if err != nil {
				return err
			}
			client, err := openBrightspace(cmd)
			if err != nil {
				return err
			}
			topic, err := client.Topic(cmd.Context(), courseID, topicID)
			if err != nil {
				return err
			}
			body, err := client.Download(cmd.Context(), topic.URL)
			if err != nil {
				return err
			}
			defer body.Close()
			path, err := saveDownload(output, topicFileName(topic), body)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"course_id": courseID, "topic_id": topicID, "path": path})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", ".", "directory for the downloaded file")
	return cmd
}

func saveDownload(directory, name string, body io.Reader) (string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return "", fmt.Errorf("create download: %w", err)
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(path)
		}
	}()
	if _, err := io.Copy(file, body); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	succeeded = true
	return path, nil
}

func safeFileName(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '.' || character == '-' || character == '_' || character == ' ' {
			return character
		}
		return '_'
	}, value)
	value = strings.Trim(value, " .")
	if value == "" {
		return "download"
	}
	return value
}

func topicFileName(topic brightspace.Topic) string {
	name := safeFileName(topic.Title)
	parsed, err := url.Parse(topic.URL)
	if err != nil {
		return name
	}
	extension := filepath.Ext(parsed.Path)
	if extension != "" && !strings.EqualFold(filepath.Ext(name), extension) {
		return name + extension
	}
	return name
}

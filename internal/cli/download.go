package cli

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/nsigel/usc-cli/internal/brightspace"
	"github.com/spf13/cobra"
)

func downloadCommand() *cobra.Command {
	var markdown bool
	var ocr bool
	var output string
	cmd := &cobra.Command{
		Use:   "download COURSE_ID TOPIC_ID",
		Short: "Download a Brightspace content item",
		Long:  "Download a Brightspace content item. PDF is the default; use --markdown for searchable, AI-friendly Markdown.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if ocr && !markdown {
				return errors.New("--ocr requires --markdown")
			}
			course, err := courseID(args[0])
			if err != nil {
				return err
			}
			topicID, err := courseID(args[1])
			if err != nil {
				return fmt.Errorf("TOPIC_ID must be a positive integer")
			}
			client, err := openBrightspace(cmd.Context())
			if err != nil {
				return err
			}
			topic, err := client.Topic(cmd.Context(), course, topicID)
			if err != nil {
				return err
			}
			if markdown && !strings.HasSuffix(strings.ToLower(topic.URL), ".pdf") {
				return fmt.Errorf("--markdown requires a PDF topic; %q is not a PDF", topic.Title)
			}
			body, err := client.Download(cmd.Context(), topic.URL)
			if err != nil {
				return err
			}
			defer body.Close()
			name := topicFileName(topic)
			if !markdown {
				path, err := saveDownload(output, name, body)
				if err != nil {
					return err
				}
				return writeJSON(cmd, map[string]any{"course_id": course, "topic_id": topicID, "format": "pdf", "path": path})
			}
			path, err := convertToMarkdown(cmd, output, name, body, ocr)
			if err != nil {
				return err
			}
			return writeJSON(cmd, map[string]any{"course_id": course, "topic_id": topicID, "format": "markdown", "path": path})
		},
	}
	cmd.Flags().BoolVar(&markdown, "markdown", false, "convert a PDF to Markdown with Docling (recommended for searchable notes)")
	cmd.Flags().BoolVar(&ocr, "ocr", false, "use OCR when converting a PDF to Markdown")
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

func convertToMarkdown(cmd *cobra.Command, directory, name string, body io.Reader, ocr bool) (string, error) {
	docling, err := findDocling()
	if err != nil {
		return "", withAction(errors.New("Docling is required for --markdown"), "pip install docling")
	}
	temporary, err := os.MkdirTemp("", "usc-docling-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	pdfPath, err := saveDownload(temporary, name, body)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	markdownPath := filepath.Join(directory, strings.TrimSuffix(name, filepath.Ext(name))+".md")
	if _, err := os.Stat(markdownPath); err == nil {
		return "", fmt.Errorf("create download: %s already exists", markdownPath)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	arguments := []string{"convert", pdfPath, "--to", "md", "--output", directory}
	if !ocr {
		arguments = append(arguments, "--no-ocr")
	}
	process := exec.CommandContext(cmd.Context(), docling, arguments...)
	if output, err := process.CombinedOutput(); err != nil {
		return "", fmt.Errorf("convert PDF with Docling: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(markdownPath); err != nil {
		return "", fmt.Errorf("Docling did not create Markdown output: %w", err)
	}
	return markdownPath, nil
}

func findDocling() (string, error) {
	if path, err := exec.LookPath("docling"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".local", "bin", "docling")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", errors.New("Docling not found")
	}
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

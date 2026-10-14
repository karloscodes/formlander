package forms

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// File upload constraints
const (
	MaxFileSize      = 10 * 1024 * 1024 // 10MB
	MaxFilesPerField = 5
	MaxTotalFiles    = 10
)

// Allowed file extensions (lowercase, with dot)
var allowedExtensions = map[string]bool{
	// Images
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".svg":  true,
	// Documents
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".xls":  true,
	".xlsx": true,
	".ppt":  true,
	".pptx": true,
	".txt":  true,
	".csv":  true,
	".rtf":  true,
	".odt":  true,
	".ods":  true,
	".odp":  true,
}

// UploadedFile represents a file ready to be saved
type UploadedFile struct {
	FieldName   string
	Filename    string
	ContentType string
	Size        int64
	Data        io.Reader
}

// ValidateFile checks if a file meets upload requirements
func ValidateFile(header *multipart.FileHeader) error {
	if header.Size > MaxFileSize {
		return fmt.Errorf("file %q exceeds maximum size of 10MB", header.Filename)
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		return fmt.Errorf("file %q has no extension", header.Filename)
	}

	if !allowedExtensions[ext] {
		return fmt.Errorf("file type %q not allowed", ext)
	}

	return nil
}

// ExtractFiles extracts and validates files from multipart form
func ExtractFiles(form *multipart.Form) ([]*UploadedFile, error) {
	if form == nil || form.File == nil {
		return nil, nil
	}

	var files []*UploadedFile
	totalFiles := 0

	for fieldName, headers := range form.File {
		if len(headers) > MaxFilesPerField {
			return nil, fmt.Errorf("too many files for field %q (max %d)", fieldName, MaxFilesPerField)
		}

		for _, header := range headers {
			totalFiles++
			if totalFiles > MaxTotalFiles {
				return nil, fmt.Errorf("too many files (max %d)", MaxTotalFiles)
			}

			if err := ValidateFile(header); err != nil {
				return nil, err
			}

			file, err := header.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open file %q: %w", header.Filename, err)
			}

			files = append(files, &UploadedFile{
				FieldName:   fieldName,
				Filename:    sanitizeFilename(header.Filename),
				ContentType: header.Header.Get("Content-Type"),
				Size:        header.Size,
				Data:        file,
			})
		}
	}

	return files, nil
}

// StageFiles writes uploads to a staging directory before the submission
// exists, so the write transaction only has to move them: disk writes under
// the write lock would make every other writer wait. It returns the staging
// directory and one record per file; MoveStagedFiles fills in the rest.
func StageFiles(dataDir string, files []*UploadedFile) (string, []*SubmissionFile, error) {
	if len(files) == 0 {
		return "", nil, nil
	}

	stagingRoot := filepath.Join(dataDir, "uploads", ".staging")
	if err := os.MkdirAll(stagingRoot, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create staging directory: %w", err)
	}
	stagingDir, err := os.MkdirTemp(stagingRoot, "submission-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create staging directory: %w", err)
	}

	var records []*SubmissionFile
	for _, f := range files {
		// Generate unique filename to avoid collisions
		filename := uniqueFilename(f.Filename)
		filePath := filepath.Join(stagingDir, filename)

		dst, err := os.Create(filePath)
		if err != nil {
			os.RemoveAll(stagingDir)
			return "", nil, fmt.Errorf("failed to create file: %w", err)
		}

		written, err := io.Copy(dst, f.Data)
		dst.Close()

		// Close the source if it's a closer
		if closer, ok := f.Data.(io.Closer); ok {
			closer.Close()
		}

		if err != nil {
			os.RemoveAll(stagingDir)
			return "", nil, fmt.Errorf("failed to save file: %w", err)
		}

		records = append(records, &SubmissionFile{
			FieldName:   f.FieldName,
			Filename:    f.Filename, // Original filename for display
			ContentType: f.ContentType,
			Size:        written,
			StoragePath: filename, // the name inside stagingDir until MoveStagedFiles
		})
	}

	return stagingDir, records, nil
}

// MoveStagedFiles moves staged files into storage/uploads/{form_id}/{submission_id}/
// and sets their submission ID and storage path. A rename on the same disk
// is quick, so it can run inside the write transaction. The directory can
// exist already: SQLite reuses the ID of a deleted last row.
func MoveStagedFiles(dataDir, stagingDir string, formID, submissionID uint, records []*SubmissionFile) error {
	relativeDir := filepath.Join("uploads", fmt.Sprintf("%d", formID), fmt.Sprintf("%d", submissionID))
	if err := os.MkdirAll(filepath.Join(dataDir, relativeDir), 0755); err != nil {
		return fmt.Errorf("failed to create upload directory: %w", err)
	}
	for _, record := range records {
		relativePath := filepath.Join(relativeDir, record.StoragePath)
		if err := os.Rename(filepath.Join(stagingDir, record.StoragePath), filepath.Join(dataDir, relativePath)); err != nil {
			return fmt.Errorf("failed to move file: %w", err)
		}
		record.SubmissionID = submissionID
		record.StoragePath = relativePath
	}
	return os.Remove(stagingDir)
}

// GetFilePath returns the full filesystem path for a submission file
func GetFilePath(dataDir string, file *SubmissionFile) string {
	return filepath.Join(dataDir, file.StoragePath)
}

// DeleteSubmissionFiles removes all files for a submission from disk
func DeleteSubmissionFiles(dataDir string, formID, submissionID uint) error {
	uploadDir := filepath.Join(dataDir, "uploads", fmt.Sprintf("%d", formID), fmt.Sprintf("%d", submissionID))
	return os.RemoveAll(uploadDir)
}

// sanitizeFilename removes dangerous characters from filename
func sanitizeFilename(name string) string {
	// Get just the base name (no path components)
	name = filepath.Base(name)

	// Replace dangerous characters
	replacer := strings.NewReplacer(
		"..", "_",
		"/", "_",
		"\\", "_",
		"\x00", "_",
	)
	name = replacer.Replace(name)

	if name == "" || name == "." {
		return "unnamed"
	}

	return name
}

// uniqueFilename adds a random suffix to prevent collisions
func uniqueFilename(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	// Generate short random suffix
	suffix := GeneratePublicID()[:8]

	return fmt.Sprintf("%s_%s%s", base, suffix, ext)
}

// CloseFiles closes all file readers (call on error paths)
func CloseFiles(files []*UploadedFile) {
	for _, f := range files {
		if closer, ok := f.Data.(io.Closer); ok {
			closer.Close()
		}
	}
}

var ErrFileNotFound = errors.New("file not found")

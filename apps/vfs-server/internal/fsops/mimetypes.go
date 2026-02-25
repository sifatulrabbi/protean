package fsops

import (
	"path/filepath"
	"strings"
)

var mimeMap = map[string]string{
	"txt":  "text/plain",
	"md":   "text/markdown",
	"json": "application/json",
	"csv":  "text/csv",
	"html": "text/html",
	"yaml": "text/yaml",
	"yml":  "text/yaml",
	"xml":  "text/xml",
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"gif":  "image/gif",
	"webp": "image/webp",
	"svg":  "image/svg+xml",
	"pdf":  "application/pdf",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"zip":  "application/zip",
}

// GetMimeType returns the MIME type for a file based on its extension.
func GetMimeType(filePath string) string {
	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	ext = strings.ToLower(ext)
	if mime, ok := mimeMap[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

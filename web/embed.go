package web

import (
	"embed"
	"io/fs"
)

//go:embed templates
var templatesFS embed.FS

// Templates provides access to embedded templates with the 'templates' prefix stripped
var Templates fs.FS

func init() {
	var err error
	Templates, err = fs.Sub(templatesFS, "templates")
	if err != nil {
		panic(err)
	}
}

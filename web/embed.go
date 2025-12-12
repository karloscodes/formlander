package web

import (
	"embed"
	"io/fs"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// Templates provides access to embedded templates with the 'templates' prefix stripped
var Templates fs.FS

// Static provides access to embedded static assets with the 'static' prefix stripped
var Static fs.FS

func init() {
	var err error
	Templates, err = fs.Sub(templatesFS, "templates")
	if err != nil {
		panic(err)
	}
	Static, err = fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
}

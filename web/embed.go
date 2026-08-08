package web

import "embed"

//go:embed index.html app.js style.css login.html login.js theme.js favicon.svg icon-512.png apple-touch-icon.png
var FS embed.FS

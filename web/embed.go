package web

import "embed"

//go:embed index.html app.js style.css login.html login.js
var FS embed.FS

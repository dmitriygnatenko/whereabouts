// Package wherewhat holds only the embedded frontend — go:embed patterns can't reach outside the
// directory of the file that declares them, so this has to live at the module root next to web/,
// separate from the actual binary entrypoint in cmd/whereabouts.
package wherewhat

import "embed"

// WebFiles is the built frontend, embedded into the binary. web/files/ is deliberately not included
// here — it's the runtime directory for uploaded photos (see internal/adapter/filesystem/storage),
// served straight off disk via a dedicated route instead of from this compiled-in snapshot.
// Embedding it wholesale would break the build the moment that directory is empty on disk (go:embed
// can't embed an empty directory).
//
//go:embed web/index.html web/styles.css web/api.js web/i18n.js web/app.js
//go:embed web/favicon.ico web/favicon.svg web/apple-touch-icon.png
var WebFiles embed.FS

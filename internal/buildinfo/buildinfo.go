// Package buildinfo exposes build metadata for runtime diagnostics.
package buildinfo

// Version is overridden during release builds with -ldflags "-X gateway/internal/buildinfo.Version=<version>".
var Version = "dev"

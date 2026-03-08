package version

var (
	// Version is the release version, injected via ldflags at build time.
	Version = "dev"
	// Commit is the git commit SHA, injected via ldflags at build time.
	Commit = "none"
	// Date is the build date, injected via ldflags at build time.
	Date = "unknown"
)

// Full returns a formatted version string.
func Full() string {
	return Version + " (" + Commit + ") " + Date
}

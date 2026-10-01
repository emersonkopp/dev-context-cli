// Package provider defines the interface that every AI tool provider must implement.
// Adding support for a new tool (Copilot, Cursor, etc.) is done by creating a
// new package under providers/ that implements this interface.
package provider

// ArtifactStatus describes whether a single artifact is current or needs update.
type ArtifactStatus struct {
	// Name is a human-readable identifier (e.g. "kiro/steering/testing-policy.md").
	Name string
	// Installed reports whether the artifact is present at the destination.
	Installed bool
	// UpToDate reports whether the installed artifact matches the source.
	UpToDate bool
	// SrcPath is the source path inside the monorepo.
	SrcPath string
	// DstPath is the destination path on the local machine.
	DstPath string
}

// Provider is the contract every AI tool integration must satisfy.
type Provider interface {
	// Name returns the human-readable tool name (e.g. "Kiro").
	Name() string

	// Install copies all artifacts from the monorepo into their global
	// destinations. It is idempotent: running it twice is safe.
	Install(repoPath string) error

	// Status reports the installation state of every artifact managed by
	// this provider.
	Status(repoPath string) ([]ArtifactStatus, error)
}

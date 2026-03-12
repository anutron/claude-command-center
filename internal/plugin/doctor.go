package plugin

// ValidationResult describes the outcome of a single doctor check.
type ValidationResult struct {
	Status  string // "ok", "missing", "incomplete", "no_client"
	Message string
	Hint    string
}

// DoctorCheck is a named check result with an optional inconclusive flag.
type DoctorCheck struct {
	Name         string
	Result       ValidationResult
	Inconclusive bool
}

// DoctorOpts controls how doctor checks are performed.
type DoctorOpts struct {
	Live bool // hit network endpoints (tokeninfo, gh auth, etc.)
}

// DoctorProvider is an optional interface for plugins that can diagnose
// their own credential and configuration health.
type DoctorProvider interface {
	DoctorChecks(opts DoctorOpts) []DoctorCheck
}

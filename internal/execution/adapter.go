package execution

// Adapter owns one vendor's command construction and normal-exit convention.
type Adapter interface {
	Command(workdir, instruction string) (name string, args []string)
	Classify(exitCode int) Outcome
}

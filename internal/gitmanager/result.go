package gitmanager

// GitApplyOutputSchema is the schema of the apply command's JSON output.
const GitApplyOutputSchema = "ai-dev.git-apply-output/v1"

// EncodeApplyOutput renders the GitApplyOutput object: exactly the
// validated result, whose status, reason, durable state, exit code and
// ordered secondary failures the model already ties together (T068).
func EncodeApplyOutput(result GitApplyResult) ([]byte, error) {
	value, err := applyResultValue(result)
	if err != nil {
		return nil, err
	}
	return MarshalCanonical(map[string]any{"schema_version": GitApplyOutputSchema, "result": value})
}

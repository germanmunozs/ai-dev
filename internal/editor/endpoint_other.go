//go:build !darwin && !linux

package editor

// CheckEndpoint refuses every endpoint on platforms ai-dev open does not
// support (research R-03).
func CheckEndpoint(string) error {
	return &EndpointError{Reason: ReasonPlatformUnsupported}
}

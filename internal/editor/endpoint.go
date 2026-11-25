package editor

// Refusal reasons for the $NVIM endpoint and the parent instance
// (data-model.md Editor Target).
const (
	ReasonNotLocalSocket      = "not_local_socket"
	ReasonNotSocket           = "not_socket"
	ReasonNotOwned            = "not_owned"
	ReasonNoAnswer            = "no_answer"
	ReasonNotNeovim           = "not_neovim"
	ReasonVersionUnsupported  = "version_unsupported"
	ReasonEditorError         = "editor_error"
	ReasonPlatformUnsupported = "platform_unsupported"
)

// EndpointError refuses the parent instance with one of the reasons above.
// Message is Neovim's own error, raw, for editor_error only; callers
// sanitize it before display.
type EndpointError struct {
	Reason  string
	Message string
}

func (e *EndpointError) Error() string {
	if e.Message != "" {
		return "editor refused: " + e.Reason + ": " + e.Message
	}
	return "editor refused: " + e.Reason
}

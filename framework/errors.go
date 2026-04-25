package framework

type ToolError struct {
	Code string `json:"code"`

	Message string `json:"message"`

	Detail string `json:"detail,omitempty"`

	Retryable bool `json:"retryable,omitempty"`
}

const (
	ErrCodeInvalidArgs      = "INVALID_ARGS"
	ErrCodeConnectionFailed = "CONNECTION_FAILED"
	ErrCodePolicyDenied     = "POLICY_DENIED"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodePermissionDenied = "PERMISSION_DENIED"
	ErrCodeUnsupported      = "UNSUPPORTED"
	ErrCodeInternalError    = "INTERNAL_ERROR"
)

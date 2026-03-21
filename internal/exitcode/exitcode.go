package exitcode

const (
	OK         = 0
	Failure    = 1
	UsageError = 2
	NotFound   = 3
	Forbidden  = 4
	Conflict   = 5
)

// Error is an error that carries an exit code.
// Resource packages return this; the CLI layer reads the code via ExitCode().
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) ExitCode() int { return e.Code }

// New wraps err with an exit code.
func New(code int, err error) error {
	return &Error{Code: code, Err: err}
}

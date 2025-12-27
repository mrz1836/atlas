package errors

import "fmt"

// Wrap adds context to errors at package boundaries.
// It returns nil if err is nil, allowing for safe inline usage.
//
// The wrapped error preserves the original error chain, enabling
// errors.Is() checks to continue working:
//
//	if err := doSomething(); err != nil {
//	    return errors.Wrap(err, "failed to do something")
//	}
//
// Callers can still check for sentinel errors:
//
//	if errors.Is(err, errors.ErrGitOperation) {
//	    // Handle git-specific error
//	}
//
// IMPORTANT: Only wrap errors at package boundaries to avoid
// overly nested error messages.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Wrapf adds formatted context to errors at package boundaries.
// It returns nil if err is nil, allowing for safe inline usage.
//
// This is useful when the context message needs variable interpolation:
//
//	return errors.Wrapf(err, "failed to process task %s", taskID)
//
// Like Wrap, the wrapped error preserves the original error chain.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", msg, err)
}

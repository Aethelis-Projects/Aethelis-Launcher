package downloader

import (
	"errors"
	"fmt"
)

var (
	ErrQueueEmpty        = errors.New("download queue is empty")
	ErrDispatcherClosed  = errors.New("dispatcher is closed")
	ErrAllMirrorsFailed  = errors.New("all mirrors failed to download asset")
	ErrInvalidStatusCode = errors.New("unexpected HTTP status code")
)

type ChecksumMismatchError struct {
	Algorithm string
	Expected  string
	Actual    string
	FilePath  string
}

func (e *ChecksumMismatchError) Error() string {
	return fmt.Sprintf("%s checksum mismatch for %s: expected %s, got %s", e.Algorithm, e.FilePath, e.Expected, e.Actual)
}
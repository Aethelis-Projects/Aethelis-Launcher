package wails

// SetOpenPathExecForTesting temporarily overrides openPathExec for testing and returns a cleanup func.
func SetOpenPathExecForTesting(fn func(cleanPath string, isDir bool) error) func() {
	prev := openPathExec
	openPathExec = fn
	return func() {
		openPathExec = prev
	}
}

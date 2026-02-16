package tools

const maxOutputBytes = 10_000

func truncate(b []byte) string {
	if len(b) > maxOutputBytes {
		return string(b[:maxOutputBytes]) + "\n... (truncated)"
	}
	return string(b)
}

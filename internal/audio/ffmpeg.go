package audio

import (
	"fmt"
	"os/exec"
)

// Installed reports whether ffmpeg is available on PATH.
func Installed() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// ConvertToWav converts input to a 16 kHz mono WAV file using ffmpeg.
func ConvertToWav(input, output string) error {
	cmd := exec.Command("ffmpeg", "-i", input, "-ar", "16000", "-ac", "1", "-f", "wav", "-y", output)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, out)
	}
	return nil
}

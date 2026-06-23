package agent

import "strings"

// stripModelPreamble removes any text the model emitted before the first
// markdown section heading ("## "). The review's document title is rendered by
// the UI layer, and every style prompt is instructed to start at its first
// "## " section — so anything before that heading is process narration or
// preamble (e.g. "Ich schreibe jetzt das Review…", a stray "# Code-Review"
// title) that must never reach the review. This is a deterministic guarantee
// independent of prompt compliance.
//
// If no "## " heading exists (e.g. the "no reviewable objects" fallback
// message), the input is returned unchanged.
func stripModelPreamble(md string) string {
	lines := strings.Split(md, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return md
}

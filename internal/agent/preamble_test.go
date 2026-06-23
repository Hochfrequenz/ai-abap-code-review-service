package agent

import "testing"

func TestStripModelPreamble(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips process narration before first section",
			in:   "Ich habe nun alle Quelltexte gesammelt. Das ATC-Tool konnte nicht ausgeführt werden. Ich schreibe jetzt das Review.\n\n## Zusammenfassung\nAlles gut.",
			want: "## Zusammenfassung\nAlles gut.",
		},
		{
			name: "strips a stray model title too",
			in:   "# Code-Review: NPLK900014\n\n## ATC-Befunde\nKeine.",
			want: "## ATC-Befunde\nKeine.",
		},
		{
			name: "keeps everything from the first section onward",
			in:   "## A\nx\n## B\ny",
			want: "## A\nx\n## B\ny",
		},
		{
			name: "unchanged when no section heading present",
			in:   "Keine prüfbaren Quellobjekte im Transport.",
			want: "Keine prüfbaren Quellobjekte im Transport.",
		},
		{
			name: "anchors on h2 only, not h3",
			in:   "preamble\n### sub\n## real\nbody",
			want: "## real\nbody",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripModelPreamble(tt.in); got != tt.want {
				t.Errorf("stripModelPreamble()\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

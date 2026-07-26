package widget

import "testing"

func TestParseEvents(t *testing.T) {
	wikitext := "{{Zijbalk maandkalender zonder week|7}}\n" +
		"'''26 juli''' is de 207de [[dag]] van het [[jaar]].\n\n" +
		"== Gebeurtenissen ==\n" +
		"* {{Kopje dag algemeen}}\n" +
		"** [[1184]] - Bij het [[latrine-incident van Erfurt]] komen mogelijk zo'n zestig mensen om het leven.\n" +
		"** [[1953]] – [[Fidel Castro]] leidt een aanval op de [[Moncadakazerne]].<ref>bron</ref>\n" +
		"** [[2016]] - [[Hillary Clinton]] wordt genomineerd.\n" +
		"\n== Geboren ==\n" +
		"** [[1875]] - [[Carl Jung]], Zwitsers psychiater\n"

	got := parseEvents(wikitext)
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(got), got)
	}
	want0 := "1184 – Bij het latrine-incident van Erfurt komen mogelijk zo'n zestig mensen om het leven."
	if got[0] != want0 {
		t.Errorf("event[0] = %q, want %q", got[0], want0)
	}
	if got[1] != "1953 – Fidel Castro leidt een aanval op de Moncadakazerne." {
		t.Errorf("event[1] ref not stripped: %q", got[1])
	}
}

package qbit

import "testing"

func TestSetINI(t *testing.T) {
	const qb = "[BitTorrent]\r\nSession\\Port=47653\r\n\r\n[Preferences]\r\nWebUI\\Port=8080\r\nWebUI\\Enabled=false\r\n\r\n[Meta]\r\nMigrationVersion=8\r\n"
	for _, c := range []struct{ name, text, section, key, value, want string }{
		{"replace", qb, "Preferences", `WebUI\Port`, "18089",
			"[BitTorrent]\nSession\\Port=47653\n\n[Preferences]\nWebUI\\Port=18089\nWebUI\\Enabled=false\n\n[Meta]\nMigrationVersion=8\n"},
		{"add to a section", qb, "Preferences", `WebUI\Address`, "127.0.0.1",
			"[BitTorrent]\nSession\\Port=47653\n\n[Preferences]\nWebUI\\Port=8080\nWebUI\\Enabled=false\nWebUI\\Address=127.0.0.1\n\n[Meta]\nMigrationVersion=8\n"},
		{"new section", qb, "LegalNotice", "Accepted", "true",
			"[BitTorrent]\nSession\\Port=47653\n\n[Preferences]\nWebUI\\Port=8080\nWebUI\\Enabled=false\n\n[Meta]\nMigrationVersion=8\n\n[LegalNotice]\nAccepted=true\n"},
		{"empty file", "", "GUI", "StartUpWindowState", "Hidden", "[GUI]\nStartUpWindowState=Hidden\n"},
		{"same key in another section", "[A]\nk=1\n[B]\nk=2\n", "B", "k", "3", "[A]\nk=1\n[B]\nk=3\n"},
		{"last section", "[A]\nk=1", "A", "j", "2", "[A]\nk=1\nj=2"},
	} {
		if got := setINI(c.text, c.section, c.key, c.value); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

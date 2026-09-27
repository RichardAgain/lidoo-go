package odoo

import "testing"

func TestDeriveRestoreDestination(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		existing []Database
		want     string
	}{
		{name: "from file name", source: "/home/me/Downloads/IPM Backup.zip", want: "ipm_backup"},
		{name: "collision adds suffix", source: "/tmp/smoke_db.dump", existing: []Database{{Logical: "smoke_db", Physical: "smoke__smoke_db"}}, want: "smoke_db_restore"},
		{name: "second collision numbers", source: "/tmp/smoke_db.dump", existing: []Database{{Logical: "smoke_db"}, {Physical: "smoke_db_restore"}}, want: "smoke_db_restore_2"},
		{name: "empty base falls back", source: "/tmp/---.zip", want: "restored"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DeriveRestoreDestination(test.source, test.existing); got != test.want {
				t.Fatalf("DeriveRestoreDestination(%q) = %q, want %q", test.source, got, test.want)
			}
		})
	}
}

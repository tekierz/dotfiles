package safefile

import "testing"

func TestRestorableOwnerPolicy(t *testing.T) {
	tests := []struct {
		name       string
		uid, gid   uint32
		euid, egid int
		want       bool
	}{
		{name: "exact owner", uid: 501, gid: 20, euid: 501, egid: 20, want: true},
		{name: "foreign uid", uid: 502, gid: 20, euid: 501, egid: 20},
		{name: "foreign gid", uid: 501, gid: 80, euid: 501, egid: 20},
		{name: "unknown uid", uid: 501, gid: 20, euid: -1, egid: 20},
		{name: "unknown gid", uid: 501, gid: 20, euid: 501, egid: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := restorableOwner(test.uid, test.gid, test.euid, test.egid); got != test.want {
				t.Fatalf("restorableOwner(%d, %d, %d, %d) = %t, want %t", test.uid, test.gid, test.euid, test.egid, got, test.want)
			}
		})
	}
}

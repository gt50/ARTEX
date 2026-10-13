package db

import (
	"fmt"
	"testing"
	"time"
)

// InsertSettingIfAbsent is a fallback for auth.password_hash: the caller's GetSetting check may
// Invalid due to database error, or may be queued by concurrent requests (bcrypt takes tens of milliseconds to run), so "only for the first time"
// The guarantee of "settable" must fall on the primary key constraint, not the upper-level if judgment.
func TestInsertSettingIfAbsentDoesNotOverwrite(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer d.Close()

	// Use the test-specific key and never touch the real auth.password_hash in the development database.
	key := fmt.Sprintf("test.insert_if_absent.%d", time.Now().UnixNano())
	defer func() { _, _ = d.Exec(`DELETE FROM settings WHERE key=$1`, key) }()

	inserted, err := d.InsertSettingIfAbsent(key, "first")
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("The first write should return inserted=true")
	}

	inserted, err = d.InsertSettingIfAbsent(key, "second")
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("Should return inserted=false when the key already exists")
	}

	got, ok, err := d.GetSetting(key)
	if err != nil || !ok {
		t.Fatalf("GetSetting: ok=%v err=%v", ok, err)
	}
	if got != "first" {
		t.Fatalf("Value is overwritten to %q and should remain %q", got, "first")
	}
}

package notehistory

import (
	"context"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecordIgnoresDuplicates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rows := []Notification{
		{UUID: "a", DeliveredAt: "2026-01-01 10:00:00", App: "com.apple.mail", Title: "one"},
		{UUID: "b", DeliveredAt: "2026-01-01 11:00:00", App: "com.apple.mail", Title: "two"},
	}
	added, err := s.Record(ctx, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 {
		t.Fatalf("first record added %d, want 2", len(added))
	}
	if added[0].RecordedAt == "" {
		t.Error("RecordedAt not set on added rows")
	}

	rows = append(rows, Notification{UUID: "c", DeliveredAt: "2026-01-01 12:00:00", Title: "three"})
	added, err = s.Record(ctx, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].UUID != "c" {
		t.Fatalf("second record added %+v, want just c", added)
	}

	t.Run("recent is newest first", func(t *testing.T) {
		got, err := s.Recent(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].UUID != "c" || got[1].UUID != "b" {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestMigrateAdoptsPythonSchema(t *testing.T) {
	// The Python script created the table without a schema_version row;
	// opening that file must not fail or lose data.
	path := t.TempDir() + "/old.db"
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE schema_version`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO notifications (uuid, title) VALUES ('old', 'kept')`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	got, err := s.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "kept" {
		t.Fatalf("got %+v", got)
	}
}

func TestPruneKeepsRecentAndUndated(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, err := s.Record(ctx, []Notification{
		{UUID: "old", DeliveredAt: "2020-01-01 00:00:00"},
		{UUID: "new", DeliveredAt: "2030-01-01 00:00:00"},
		{UUID: "undated"},
	})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := s.Prune(ctx, time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if gone != 1 {
		t.Fatalf("pruned %d, want 1", gone)
	}
	got, _ := s.All(ctx)
	if len(got) != 2 {
		t.Fatalf("left %+v", got)
	}
	for _, n := range got {
		if n.UUID == "old" {
			t.Error("old row survived")
		}
	}
}

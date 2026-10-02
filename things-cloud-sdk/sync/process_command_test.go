package sync

import (
	"encoding/json"
	"path/filepath"
	"testing"

	things "github.com/arthursoares/things-cloud-sdk"
)

// Mail to Things queue entries carry no task-graph state and must not surface
// as UnknownChange (which would log, and could expose, the email body).
func TestProcessMailQueueCommands(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	syncer, err := Open(dbPath, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer syncer.Close()

	changes, err := syncer.processItems([]things.Item{
		{UUID: "8F63FEC2-1202-43D8-80D7-55B4AC4FCA37", Kind: things.ItemKindCommand, Action: things.ItemActionCreated,
			P: json.RawMessage(`{"cd":1785223359,"tp":0,"if":{"id":"E9EBA235-0000-0000-0000-000000000000","nt":"<note>body</note>"}}`)},
		{UUID: "457jbco4TLEAXyjgnrgE6X", Kind: things.ItemKindCommand3, Action: things.ItemActionDeleted, P: json.RawMessage(`{}`)},
	}, 0)
	if err != nil {
		t.Fatalf("processItems failed: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes for mail queue commands, got %#v", changes)
	}
}

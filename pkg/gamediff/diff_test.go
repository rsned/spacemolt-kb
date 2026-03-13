package gamediff

import (
	"testing"
)

func TestDiffCatalog_Additions(t *testing.T) {
	old := []byte(`{"items":[{"id":"a","name":"Alpha"}]}`)
	new := []byte(`{"items":[{"id":"a","name":"Alpha"},{"id":"b","name":"Beta"}]}`)

	result, err := DiffCatalog(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Additions) != 1 {
		t.Fatalf("want 1 addition, got %d", len(result.Additions))
	}
	if result.Additions[0].ID != "b" {
		t.Errorf("want addition id=b, got %s", result.Additions[0].ID)
	}
}

func TestDiffCatalog_Deletions(t *testing.T) {
	old := []byte(`{"items":[{"id":"a","name":"Alpha"},{"id":"b","name":"Beta"}]}`)
	new := []byte(`{"items":[{"id":"a","name":"Alpha"}]}`)

	result, err := DiffCatalog(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deletions) != 1 {
		t.Fatalf("want 1 deletion, got %d", len(result.Deletions))
	}
	if result.Deletions[0].ID != "b" {
		t.Errorf("want deletion id=b, got %s", result.Deletions[0].ID)
	}
}

func TestDiffCatalog_Modifications(t *testing.T) {
	old := []byte(`{"items":[{"id":"a","name":"Alpha","value":10}]}`)
	new := []byte(`{"items":[{"id":"a","name":"Alpha","value":20}]}`)

	result, err := DiffCatalog(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Field != "value" {
		t.Errorf("want field=value, got %s", result.Changes[0].Field)
	}
}

func TestDiffCatalog_NoChanges(t *testing.T) {
	data := []byte(`{"items":[{"id":"a","name":"Alpha"}]}`)

	result, err := DiffCatalog(data, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Additions) != 0 || len(result.Deletions) != 0 || len(result.Changes) != 0 {
		t.Error("expected no changes")
	}
}

func TestDiffMap_Additions(t *testing.T) {
	old := []byte(`{"systems":[{"system_id":"sol","name":"Sol"}]}`)
	new := []byte(`{"systems":[{"system_id":"sol","name":"Sol"},{"system_id":"nexus","name":"Nexus"}]}`)

	result, err := DiffMap(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Additions) != 1 {
		t.Fatalf("want 1 addition, got %d", len(result.Additions))
	}
	if result.Additions[0].ID != "nexus" {
		t.Errorf("want addition id=nexus, got %s", result.Additions[0].ID)
	}
}

func TestDiffMap_Modifications(t *testing.T) {
	old := []byte(`{"systems":[{"system_id":"sol","name":"Sol","security":80}]}`)
	new := []byte(`{"systems":[{"system_id":"sol","name":"Sol","security":100}]}`)

	result, err := DiffMap(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Field != "security" {
		t.Errorf("want field=security, got %s", result.Changes[0].Field)
	}
}

func TestDiffCatalog_SortsResults(t *testing.T) {
	old := []byte(`{"items":[]}`)
	new := []byte(`{"items":[{"id":"c","name":"Charlie"},{"id":"a","name":"Alpha"},{"id":"b","name":"Beta"}]}`)

	result, err := DiffCatalog(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Additions) != 3 {
		t.Fatalf("want 3 additions, got %d", len(result.Additions))
	}
	if result.Additions[0].Name != "Alpha" || result.Additions[1].Name != "Beta" || result.Additions[2].Name != "Charlie" {
		t.Errorf("additions not sorted: %v", result.Additions)
	}
}

func TestHasChanges(t *testing.T) {
	empty := &CatalogDiff{}
	if empty.HasChanges() {
		t.Error("empty diff should not have changes")
	}

	withAdd := &CatalogDiff{Additions: []Entry{{ID: "a", Name: "A"}}}
	if !withAdd.HasChanges() {
		t.Error("diff with additions should have changes")
	}
}

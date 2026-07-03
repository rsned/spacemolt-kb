package profilejson

import "testing"

func TestMigrateMissingVersion(t *testing.T) {
	_, err := Migrate([]byte(`{"type":"terran","seed":"x"}`))
	if err == nil {
		t.Errorf("Migrate accepted envelope with no schemaVersion")
	}
}

func TestMigrateUnknownVersion(t *testing.T) {
	_, err := Migrate([]byte(`{"schemaVersion":"999"}`))
	if err == nil {
		t.Errorf("Migrate accepted unknown schemaVersion")
	}
}

func TestMigrateCurrentVersionPassthrough(t *testing.T) {
	in := []byte(`{"schemaVersion":"` + CurrentSchemaVersion + `","type":"terran"}`)
	out, err := Migrate(in)
	if err != nil {
		t.Fatalf("Migrate(current): %v", err)
	}
	if string(out) != string(in) {
		t.Errorf("Migrate(current) modified payload:\n in: %s\nout: %s", in, out)
	}
}

func TestMigrateBadJSON(t *testing.T) {
	if _, err := Migrate([]byte(`not json`)); err == nil {
		t.Errorf("Migrate accepted invalid JSON")
	}
}

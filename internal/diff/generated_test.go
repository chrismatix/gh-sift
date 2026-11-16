package diff

import "testing"

func TestGeneratedByAttributes(t *testing.T) {
	attributes := "**/*.pb.go linguist-generated=true\nfrontend/generated/** linguist-generated\ngo.sum -linguist-generated\nkeep.pb.go linguist-generated=false\n"
	declared, err := GeneratedByAttributes(attributes, []string{"lib/a.pb.go", "frontend/generated/x/y.ts", "go.sum", "keep.pb.go", "main.go"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"lib/a.pb.go": true, "frontend/generated/x/y.ts": true, "go.sum": false, "keep.pb.go": false}
	if len(declared) != len(want) {
		t.Fatalf("got %v, want %v", declared, want)
	}
	for path, generated := range want {
		if declared[path] != generated {
			t.Errorf("%s: got %v, want %v", path, declared[path], generated)
		}
	}
}

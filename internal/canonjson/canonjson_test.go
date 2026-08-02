package canonjson

import "testing"

func TestMarshalMatchesPythonEnsureASCIIFalse(t *testing.T) {
	input := map[string]any{
		"html":   "<>&",
		"line":   "\u2028\u2029",
		"é":      "한글",
		"\ue000": 1,
		"😀":      2,
	}
	got, err := Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("{\"html\":\"<>&\",\"line\":\"" + "\u2028\u2029" + "\",\"é\":\"한글\",\"" + "\ue000" + "\":1,\"😀\":2}")
	if string(got) != string(want) {
		t.Fatalf("canonical JSON mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestMarshalIndentSortsNestedKeys(t *testing.T) {
	got, err := MarshalIndent(map[string]any{"z": []any{map[string]any{"b": 2, "a": 1}}, "a": true})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": true,\n  \"z\": [\n    {\n      \"a\": 1,\n      \"b\": 2\n    }\n  ]\n}"
	if string(got) != want {
		t.Fatalf("pretty JSON mismatch\n%s\n--- want ---\n%s", got, want)
	}
}

package jsonfile

import "testing"

func roundTrip(t *testing.T, in string) string {
	t.Helper()
	v, err := Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestEncodeMatchesYqLayout(t *testing.T) {
	in := `{"b":1,"a":{"x":[1,"s",true,null],"y":{}},"c":[]}`
	want := "{\n  \"b\": 1,\n  \"a\": {\n    \"x\": [\n      1,\n      \"s\",\n      true,\n      null\n    ],\n    \"y\": {}\n  },\n  \"c\": []\n}\n"
	if got := roundTrip(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Key order is the whole point; encoding/json would sort these.
func TestParsePreservesKeyOrder(t *testing.T) {
	v, err := Parse([]byte(`{"z":1,"a":2,"m":3}`))
	if err != nil {
		t.Fatal(err)
	}
	obj := v.(*Object)
	keys := obj.Keys()
	if len(keys) != 3 || keys[0] != "z" || keys[1] != "a" || keys[2] != "m" {
		t.Errorf("got %v", keys)
	}
}

// Numbers must come back out spelled as they went in; 1.0 and 1 are
// different bytes to the user even if equal to a parser.
func TestNumbersRoundTripVerbatim(t *testing.T) {
	if got := roundTrip(t, `{"a":1.0,"b":10000000000000000000}`); got != "{\n  \"a\": 1.0,\n  \"b\": 10000000000000000000\n}\n" {
		t.Errorf("got %q", got)
	}
}

// yq does not HTML-escape; a "<" in a hostname pattern must stay literal.
func TestNoHTMLEscaping(t *testing.T) {
	if got := roundTrip(t, `{"h":"a<b>&c"}`); got != "{\n  \"h\": \"a<b>&c\"\n}\n" {
		t.Errorf("got %q", got)
	}
}

func TestAbsentTreatsFalseAndNullAsMissing(t *testing.T) {
	if !Absent(nil, false) || !Absent(nil, true) || !Absent(false, true) {
		t.Error("nil, missing and false must all be absent")
	}
	if Absent(true, true) || Absent("", true) || Absent(0, true) {
		t.Error("true, empty string and zero are present")
	}
}

func TestUniqueStringsKeepsFirstOccurrence(t *testing.T) {
	got := UniqueStrings([]any{"b", "a"}, "a", "c", "b")
	if len(got) != 3 || got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Errorf("got %v", got)
	}
}

func TestSetDefaultDoesNotOverride(t *testing.T) {
	o := NewObject()
	o.Set("k", "user")
	SetDefault(o, "k", "default")
	SetDefault(o, "n", "default")
	if v, _ := o.Get("k"); v != "user" {
		t.Errorf("k was overridden: %v", v)
	}
	if v, _ := o.Get("n"); v != "default" {
		t.Errorf("n was not defaulted: %v", v)
	}
}

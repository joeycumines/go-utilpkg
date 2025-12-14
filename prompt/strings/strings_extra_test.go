package strings

import "testing"

func TestBasicCounts(t *testing.T) {
	if Len("abc") != 3 {
		t.Fatalf("Len mismatch")
	}
	if RuneCountInString("ππ") != 2 {
		t.Fatalf("RuneCountInString mismatch")
	}
	if RuneCount([]byte("go")) != 2 {
		t.Fatalf("RuneCount mismatch")
	}
	if GraphemeCountInString("👍🏽a") != 2 {
		t.Fatalf("GraphemeCountInString mismatch")
	}
	if GetRuneWidth('界') == 0 {
		t.Fatalf("GetRuneWidth should be positive")
	}
}

func TestIndexHelpers(t *testing.T) {
	if IndexNotByte("aaa", 'a') != -1 {
		t.Fatalf("IndexNotByte expected -1")
	}
	if LastIndexNotByte("baa", 'a') != 0 {
		t.Fatalf("LastIndexNotByte expected 0")
	}
	if IndexNotAny("abc", "abc") != -1 {
		t.Fatalf("IndexNotAny should be -1")
	}
	if LastIndexNotAny("abc", "abc") != -1 {
		t.Fatalf("LastIndexNotAny should be -1")
	}

	pos := RuneIndexNthGrapheme("go👍", 2)
	if pos == 0 {
		t.Fatalf("RuneIndexNthGrapheme should advance")
	}
	col := RuneIndexNthColumn("go👍", 2)
	if col == 0 {
		t.Fatalf("RuneIndexNthColumn should advance")
	}
}

func TestMakeASCIISetAndNotContains(t *testing.T) {
	set, ok := makeASCIISet("abc")
	if !ok {
		t.Fatalf("expected ascii set to build")
	}
	if set.notContains('a') {
		t.Fatalf("expected set to contain 'a'")
	}
	if !set.notContains('z') {
		t.Fatalf("expected set to not contain 'z'")
	}

	if _, ok := makeASCIISet("あ"); ok {
		t.Fatalf("non-ascii input should fail")
	}
}

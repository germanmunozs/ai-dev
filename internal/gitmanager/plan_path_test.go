package gitmanager

import "testing"

// F008-FR-177: a path holding '|' can never enter a plan, because the
// approval tuple PATH|MODE|sha256:...|blob:... could not represent it.
func TestPathWithPipeIsRefusedBeforeCanonicalization(t *testing.T) {
	for _, path := range []string{"a|b.txt", "dir|x/file", "|lead", "trail|"} {
		file := validAuthorizedFile("x.txt")
		file.PathUTF8, file.ReversiblePathBytesBase64URL = path, EncodeReversiblePath([]byte(path))
		if err := file.Validate(); err == nil {
			t.Errorf("authorized path %q accepted", path)
		}
		in := validIntentInput(t)
		in.AuthorizedFiles = append(in.AuthorizedFiles, file)
		if _, err := NewImmutableIntent(in); err == nil {
			t.Errorf("intent with %q accepted", path)
		}
	}
	// Other shell metacharacters stay literal and representable.
	file := validAuthorizedFile("a;b$(c)&d.txt")
	if err := file.Validate(); err != nil {
		t.Fatalf("literal metacharacters refused: %v", err)
	}
}

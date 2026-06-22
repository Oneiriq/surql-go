package schema

import "testing"

func TestFieldType_FileBytes_IsValid(t *testing.T) {
	if !FieldTypeFile.IsValid() {
		t.Error("FieldTypeFile should be valid")
	}
	if !FieldTypeBytes.IsValid() {
		t.Error("FieldTypeBytes should be valid")
	}
}

func TestFileField_Emit(t *testing.T) {
	f := FileField("avatar")
	if f.Type != FieldTypeFile {
		t.Fatalf("Type = %q, want file", f.Type)
	}
	got := f.ToSurql("user")
	want := "DEFINE FIELD avatar ON TABLE user TYPE file;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestBytesField_Emit(t *testing.T) {
	f := BytesField("blob")
	if f.Type != FieldTypeBytes {
		t.Fatalf("Type = %q, want bytes", f.Type)
	}
	got := f.ToSurql("doc")
	want := "DEFINE FIELD blob ON TABLE doc TYPE bytes;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestParseField_FileBytesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		def  string
		want FieldType
	}{
		{"DEFINE FIELD avatar ON TABLE user TYPE file", FieldTypeFile},
		{"DEFINE FIELD blob ON TABLE doc TYPE bytes", FieldTypeBytes},
	} {
		fd, err := ParseField("f", tc.def)
		if err != nil {
			t.Fatalf("ParseField(%q): %v", tc.def, err)
		}
		if fd.Type != tc.want {
			t.Errorf("ParseField(%q) type = %q, want %q", tc.def, fd.Type, tc.want)
		}
	}
}

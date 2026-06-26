package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/meyt/tapetest"
)

// readFilePart asserts the Content-Type of the first file part in a request.
func readFilePart(t *testing.T, r *http.Request) (field, filename, contentType string) {
	t.Helper()
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	for name, fhs := range r.MultipartForm.File {
		fh := fhs[0]
		in, err := fh.Open()
		if err != nil {
			t.Fatalf("open file part: %v", err)
		}
		in.Close()
		return name, fh.Filename, fh.Header.Get("Content-Type")
	}
	t.Fatal("request had no file part")
	return
}

// TestFileDefaultContentTypeIsOctetStream confirms the historical default
// behaviour: a File upload with no explicit content type is sent as
// application/octet-stream.
func TestFileDefaultContentTypeIsOctetStream(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(tmp, []byte("\x89PNG\r\n\x1a\n fake png"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotField, gotFile, gotCT string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotField, gotFile, gotCT = readFilePart(t, r)
		w.WriteHeader(http.StatusOK)
	})

	c := HandlerClient(t, handler)
	c.Post("/upload", File("avatar", tmp))

	if gotField != "avatar" {
		t.Errorf("field: want avatar, got %q", gotField)
	}
	if gotFile != "photo.png" {
		t.Errorf("filename: want photo.png, got %q", gotFile)
	}
	if gotCT != "application/octet-stream" {
		t.Errorf("content-type: want application/octet-stream, got %q", gotCT)
	}
}

// TestFileCustomContentType verifies that passing an explicit MIME type to File
// sets it on the uploaded part — the gap that blocked MIME-validating servers
// (e.g. image upload validators that reject application/octet-stream).
func TestFileCustomContentType(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(tmp, []byte("\x89PNG\r\n\x1a\n fake png"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotField, gotFile, gotCT string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotField, gotFile, gotCT = readFilePart(t, r)
		w.WriteHeader(http.StatusOK)
	})

	c := HandlerClient(t, handler)
	c.Post("/upload", File("avatar", tmp, "image/png"))

	if gotField != "avatar" || gotFile != "photo.png" {
		t.Errorf("field/file = %q/%q", gotField, gotFile)
	}
	if gotCT != "image/png" {
		t.Errorf("content-type: want image/png, got %q", gotCT)
	}
}

// TestFileCustomContentTypeMixedFormData checks the custom content type still
// works when regular form fields are present in the same multipart request.
func TestFileCustomContentTypeMixedFormData(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "pic.jpg")
	if err := os.WriteFile(tmp, []byte("fake jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	var (
		gotFormName string
		gotCT       string
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotFormName = r.MultipartForm.Value["name"][0]
		_, _, gotCT = readFilePart(t, r)
		w.WriteHeader(http.StatusOK)
	})

	c := HandlerClient(t, handler)
	c.Post("/upload", Form{"name": "John"}, File("avatar", tmp, "image/jpeg"))

	if gotFormName != "John" {
		t.Errorf("form name: want John, got %q", gotFormName)
	}
	if !strings.HasPrefix(gotCT, "image/jpeg") {
		t.Errorf("content-type: want image/jpeg, got %q", gotCT)
	}
}

package analysis

import (
	"os"
	"path/filepath"
	"testing"

	id3v2 "github.com/bogem/id3v2/v2"
)

func TestReadEmbeddedMetadataFromID3File(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "meta.mp3")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	tagFile, err := id3v2.Open(path, id3v2.Options{Parse: false})
	if err != nil {
		t.Fatalf("open id3 file: %v", err)
	}
	tagFile.SetTitle("Neon Pulse")
	tagFile.SetArtist("Unit Test")
	tagFile.AddTextFrame(tagFile.CommonID("Genre"), tagFile.DefaultEncoding(), "Cyberpunk")
	tagFile.AddCommentFrame(id3v2.CommentFrame{
		Encoding:    tagFile.DefaultEncoding(),
		Language:    "eng",
		Description: "desc",
		Text:        "dark future",
	})
	if err := tagFile.Save(); err != nil {
		t.Fatalf("save id3 file: %v", err)
	}
	if err := tagFile.Close(); err != nil {
		t.Fatalf("close id3 file: %v", err)
	}

	metadata, err := ReadEmbeddedMetadata(path)
	if err != nil {
		t.Fatalf("read embedded metadata: %v", err)
	}

	if metadata.Values["title"] != "Neon Pulse" {
		t.Fatalf("unexpected title: %q", metadata.Values["title"])
	}
	if metadata.Values["artist"] != "Unit Test" {
		t.Fatalf("unexpected artist: %q", metadata.Values["artist"])
	}
	if metadata.Values["genre"] != "Cyberpunk" {
		t.Fatalf("unexpected genre: %q", metadata.Values["genre"])
	}
}

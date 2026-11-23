package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalHashAndCancellationPreserveSavedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved.mp3")
	b := []byte("persistent audio")
	os.WriteFile(path, b, 0600)
	sum := sha256.Sum256(b)
	hash := hex.EncodeToString(sum[:])
	s, e := OpenVerifiedLocal(context.Background(), path, hash)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	if _, e = os.Stat(path); e != nil {
		t.Fatal("saved file deleted", e)
	}
	os.WriteFile(path, []byte("corrupted! audio"), 0600)
	if _, e = OpenVerifiedLocal(context.Background(), path, hash); e == nil {
		t.Fatal("corruption not detected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = OpenVerifiedLocal(ctx, path, hash); e != context.Canceled {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("cancellation deleted saved file")
	}
}

package store

import (
	"bytes"
	"testing"
	"time"
)

func TestSessionStore(t *testing.T) {
	s := newTestStore(t)
	ss := s.SessionStore()

	if _, found, err := ss.Find("missing"); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if err := ss.Commit("tok", []byte("one"), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := ss.Commit("tok", []byte("two"), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	data, found, err := ss.Find("tok")
	if err != nil || !found || !bytes.Equal(data, []byte("two")) {
		t.Fatalf("%q %v %v", data, found, err)
	}
	if err := ss.Delete("tok"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := ss.Find("tok"); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}

	if err := ss.Commit("old", []byte("x"), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, found, err := ss.Find("old"); err != nil || found {
		t.Fatalf("expired session found=%v err=%v", found, err)
	}
}

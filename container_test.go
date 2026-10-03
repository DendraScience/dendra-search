package search

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestContainerRoundTrip(t *testing.T) {
	b := NewBuilder()
	fillModes(t, b)
	pub, _, err := b.Finish(FinishPublic)
	if err != nil {
		t.Fatal(err)
	}
	other := NewBuilder()
	fillModes(t, other)
	second, _, err := other.Finish(FinishPublic)
	if err != nil {
		t.Fatal(err)
	}
	in := []ContainerChunk{
		{OrganizationID: "org-b", ETag: "etag-b", Bytes: second},
		{OrganizationID: "org-a", ETag: "etag/a", Bytes: append([]byte(nil), pub...)},
	}
	frame, err := WriteContainer(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ParseContainer(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("chunks %d", len(out))
	}
	for i := range in {
		if out[i].OrganizationID != in[i].OrganizationID || out[i].ETag != in[i].ETag || !bytes.Equal(out[i].Bytes, in[i].Bytes) {
			t.Fatalf("chunk %d %+v", i, out[i].OrganizationID)
		}
	}

	empty, err := WriteContainer(nil)
	if err != nil {
		t.Fatal(err)
	}
	none, err := ParseContainer(empty)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty: %v len %d", err, len(none))
	}

	badMagic := append([]byte(nil), frame...)
	badMagic[0] = 'X'
	if _, err := ParseContainer(badMagic); !errors.Is(err, ErrContainer) {
		t.Fatalf("bad magic: %v", err)
	}
	badVer := append([]byte(nil), frame...)
	binary.LittleEndian.PutUint16(badVer[4:], 2)
	if _, err := ParseContainer(badVer); !errors.Is(err, ErrContainer) {
		t.Fatalf("bad version: %v", err)
	}
	if _, err := ParseContainer(frame[:len(frame)-1]); !errors.Is(err, ErrContainer) {
		t.Fatalf("truncated: %v", err)
	}
	if _, err := ParseContainer(append(frame, 0)); !errors.Is(err, ErrContainer) {
		t.Fatalf("trailing: %v", err)
	}

	over := append([]byte(nil), empty...)
	binary.LittleEndian.PutUint32(over[6:], 1)
	over = append(over, 0, 0, 0, 0) // empty org id
	over = append(over, 0, 0, 0, 0) // empty etag
	var huge [4]byte
	binary.LittleEndian.PutUint32(huge[:], maxField+1)
	over = append(over, huge[:]...)
	if _, err := ParseContainer(over); !errors.Is(err, ErrContainer) {
		t.Fatalf("overlong chunk: %v", err)
	}
}

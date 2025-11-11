package webp

import (
	"bytes"
	"io"
	"testing"
)

func encodeU32(u uint32) []byte {
	return []byte{
		byte(u >> 0),
		byte(u >> 8),
		byte(u >> 16),
		byte(u >> 24),
	}
}

func TestSubChunkReader(t *testing.T) {
	for _, numSubChunks := range []int{1, 5, 10, 20} {
		chunk := make([]byte, 0, 2048)
		wantLength := uint32(128)
		for range numSubChunks {
			chunk = append(chunk, genSubChunk(wantLength)...)
		}

		r := NewSubChunkReader(bytes.NewReader(chunk))
		chunksRead := 0
		for {
			fourCC, data, dataLen, err := r.Next()
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Errorf("unexpected error: %s", err.Error())
				t.FailNow()
			}
			if dataLen != wantLength {
				t.Errorf("wanted subchunk length: %d, got %d", wantLength, dataLen)
			}
			if fourCC != [4]byte{'A', 'B', 'C', 'D'} {
				t.Errorf("fourCC was not ABCD")
			}

			dataBytes, err := io.ReadAll(data)
			if err != nil {
				t.Errorf("failed to read data from subchunk reader: %s", err.Error())
				t.FailNow()
			}
			if len(dataBytes) != int(wantLength) {
				t.Errorf("wanted datalen of %d, but got %d", wantLength, len(dataBytes))
			}

			chunksRead++
		}
		if chunksRead != numSubChunks {
			t.Errorf("expected %d subchunks, but got %d", numSubChunks, chunksRead)
		}
	}
}

func TestSubChunkReader_ChunkDataTooShort(t *testing.T) {
	// Generate a chunk, but strip some data off the end
	chunk := genSubChunk(256)[:256]
	r := NewSubChunkReader(bytes.NewReader(chunk))
	_, _, _, err := r.Next()
	if err != errInvalidFormat {
		t.Errorf("expected invalid format error, but got %v", err)
	}
}
func TestSubChunkReader_HeaderTooShort(t *testing.T) {
	// Generate a chunk, but strip some data off the end
	chunk := make([]byte, 3)
	r := NewSubChunkReader(bytes.NewReader(chunk))
	_, _, _, err := r.Next()
	if err != errInvalidHeader {
		t.Errorf("expected invalid format error, but got %v", err)
	}
}

func genSubChunk(length uint32) []byte {
	header := append([]byte("ABCD"), encodeU32(length)...)
	data := make([]byte, length)
	return append(header, data...)
}

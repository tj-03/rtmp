package message

import (
	"bufio"
	"fmt"
	"testing"

	"github.com/tj03/rtmp/src/util"
)

func TestChunkToBytes(t *testing.T) {
	chunks := []Chunk{
		{
			ChunkBasicHeader{0, 2},
			ChunkMessageHeader{0, 4, 5, 0},
			0,
			[]byte{0, 76, 75, 64}},

		{
			ChunkBasicHeader{1, 2},
			ChunkMessageHeader{0, 5, 6, 0},
			0,
			[]byte{0, 76, 75, 64, 2}}}
	data, err := ChunksToBytes(chunks)
	if err != nil {
		t.Fatalf("Chunk parsing failed")
	}
	correct1 := []byte{2, 0, 0, 0, 0, 0, 4, 5, 0, 0, 0, 0, 0, 76, 75, 64}
	correct2 := []byte{66, 0, 0, 0, 0, 0, 5, 6, 0, 76, 75, 64, 2}
	if !util.CmpSlice(data, append(correct1, correct2...)) {
		fmt.Println(data)
		t.Fatalf("Incorrect")
	}

}

func TestChunkToBytesLongMessage(t *testing.T) {
	chunks := []Chunk{
		{
			ChunkBasicHeader{0, 2},
			ChunkMessageHeader{0, 8, 5, 0},
			0,
			[]byte{69, 69}},

		{
			ChunkBasicHeader{3, 2},
			ChunkMessageHeader{0, 8, 6, 0},
			0,
			[]byte{69, 69}},
		{
			ChunkBasicHeader{3, 2},
			ChunkMessageHeader{0, 8, 6, 0},
			0,
			[]byte{69, 69}},
		{
			ChunkBasicHeader{3, 2},
			ChunkMessageHeader{0, 8, 6, 0},
			0,
			[]byte{69}}}
	data, err := ChunksToBytes(chunks)
	if err != nil {
		t.Fatalf("Chunk parsing failed")
	}
	correct1 := []byte{2, 0, 0, 0, 0, 0, 8, 5, 0, 0, 0, 0, 69, 69, 11, 69, 69, 11, 69, 69, 11, 69}
	if !util.CmpSlice(data, correct1) {
		fmt.Println(data)
		t.Fatalf("Incorrect")
	}

}

type MockReader struct {
	Data []byte
}

type MockWriter struct {
	Data []byte
}

func (reader *MockReader) Read(b []byte) (int, error) {
	size := len(b)
	if size > len(reader.Data) {
		size = len(reader.Data)
	}
	copy(b, reader.Data[:size])
	reader.Data = reader.Data[size:]
	return size, nil
}

func (reader *MockWriter) Write(b []byte) (int, error) {
	return 0, nil
}

func cmpChunks(c1 Chunk, c2 Chunk) bool {
	return c1.BasicHeader == c2.BasicHeader &&
		c2.MessageHeader == c1.MessageHeader &&
		c1.ExtendedTimestamp == c2.ExtendedTimestamp &&
		util.CmpSlice(c1.ChunkData, c2.ChunkData)

}

func TestBytesToChunk(t *testing.T) {
	correct1 := []byte{2, 0, 0, 0, 0, 0, 8, 6, 0, 0, 0, 0, 0, 76, 75, 64}
	correct2 := []byte{66, 0, 0, 0, 0, 0, 8, 6, 0, 76, 75, 64}
	bytes := append(correct1, correct2...)
	correctChunks := []Chunk{
		{
			ChunkBasicHeader{0, 2},
			ChunkMessageHeader{0, 8, 6, 0},
			0,
			[]byte{0, 76, 75, 64}},

		{
			ChunkBasicHeader{1, 2},
			ChunkMessageHeader{0, 8, 6, 0},
			0,
			[]byte{0, 76, 75, 64}}}
	mockReader := MockReader{bytes}
	conn := bufio.NewReadWriter(bufio.NewReader(&mockReader), bufio.NewWriter(&MockWriter{}))
	cStreamer := ChunkStreamer{}
	cStreamer.Init(conn, 4)
	chunk1, _, err := cStreamer.ReadChunkFromStream()
	if err != nil {
		t.Fatalf("Error reading chunk1 in cStreamer: " + err.Error())
	}
	chunk2, _, err := cStreamer.ReadChunkFromStream()
	if err != nil {
		t.Fatalf("Error reading chunk2 in cStreamer:" + err.Error())
	}
	if !cmpChunks(chunk1, correctChunks[0]) {
		fmt.Println(chunk1)
		t.Fatalf("Incorrect chunk 1")
	}
	if !cmpChunks(chunk2, correctChunks[1]) {
		fmt.Println(chunk2)
		t.Fatalf("Incorrect chunk 2")
	}

}

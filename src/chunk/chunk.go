package chunk

//might not use, think i might use it to get prev chunk fo a msg stream/chunk stream
type ChunkStreamContext struct {
}
type ChunkBasicHeader struct {
	fmt int8
	//is this the right data type?
	chunkStreamId int
}

type ChunkMessageHeader struct {
	messageType int
	//3 bytes
	messageLength int
	messageTypeId int
	//4 bytes
	messageStreamId int
	//3 bytes
	timeStamp int
}

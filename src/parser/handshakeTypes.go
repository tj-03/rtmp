package parser

const C0SIZE = 8
const C1SIZE = 1536
const C2SIZE = C1SIZE
const S0SIZE = C0SIZE
const S1SIZE = C1SIZE
const S2SIZE = S1SIZE

//todo:tf are the versions
type C0 struct {
	Version uint8
}

type S0 = C0

type C1 struct {
	Time        uint32
	Zero        uint32
	RandomBytes [1528]uint8
}

type C2 = C1
type S1 = C1
type S2 = S1

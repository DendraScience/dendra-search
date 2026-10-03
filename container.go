package search

// ContainerChunk is one organization's public shard inside an uncompressed
// DSPC frame. Bytes are stored and returned unchanged.
type ContainerChunk struct {
	OrganizationID string
	ETag           string
	Bytes          []byte
}

const containerVersion uint16 = 1

var containerMagic = [4]byte{'D', 'S', 'P', 'C'}

// WriteContainer frames chunks in the order given. It does not sort
// organizations, inspect shard magic, or decide which etag changed.
func WriteContainer(chunks []ContainerChunk) ([]byte, error) {
	var e enc
	e.b = append(e.b, containerMagic[:]...)
	e.u16(containerVersion)
	e.u32(uint32(len(chunks)))
	for _, c := range chunks {
		if len(c.OrganizationID) > maxField || len(c.ETag) > maxField || len(c.Bytes) > maxField {
			return nil, ErrContainer
		}
		e.str(c.OrganizationID)
		e.str(c.ETag)
		e.u32(uint32(len(c.Bytes)))
		e.b = append(e.b, c.Bytes...)
	}
	return e.b, nil
}

// ParseContainer reads an uncompressed DSPC frame. The chunk bytes are copies.
func ParseContainer(frame []byte) ([]ContainerChunk, error) {
	d := dec{b: frame}
	magic, err := d.bytes(4)
	if err != nil || string(magic) != string(containerMagic[:]) {
		return nil, ErrContainer
	}
	ver, err := d.u16()
	if err != nil || ver != containerVersion {
		return nil, ErrContainer
	}
	n, err := d.u32()
	if err != nil {
		return nil, ErrContainer
	}
	var chunks []ContainerChunk
	for i := uint32(0); i < n; i++ {
		org, err := d.str()
		if err != nil {
			return nil, ErrContainer
		}
		etag, err := d.str()
		if err != nil {
			return nil, ErrContainer
		}
		size, err := d.u32()
		if err != nil {
			return nil, ErrContainer
		}
		raw, err := d.bytes(int(size))
		if err != nil {
			return nil, ErrContainer
		}
		chunks = append(chunks, ContainerChunk{
			OrganizationID: org,
			ETag:           etag,
			Bytes:          append([]byte(nil), raw...),
		})
	}
	if d.i != len(d.b) {
		return nil, ErrContainer
	}
	return chunks, nil
}

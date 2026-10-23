package png_chunks

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

var pngSig = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func injectTextChunk(key, content string, stream io.Writer) {
	injectChunk("tEXt", []byte(key+"\x00"+content), stream)
}

func injectChunk(chunkName string, contents []byte, stream io.Writer) {
	length := uint32(len(contents))
	buffer := append([]byte(chunkName), contents...)
	buffer = append(buffer, make([]byte, 4)...) // Extra 4 bytes for CRC

	binary.Write(stream, binary.BigEndian, length)
	stream.Write(buffer[:len(buffer)-4])

	crcOut := pngCRC(buffer[:len(buffer)-4])
	binary.Write(stream, binary.BigEndian, crcOut)
}

var crcTable [256]uint32

// pngCRC calculates CRC-32 checksum
func pngCRC(stream []byte) uint32 {
	if crcTable[1] == 0 { // Initialize table only once
		for n := uint32(0); n <= 255; n++ {
			c := n
			for k := 0; k < 8; k++ {
				if (c & 1) == 1 {
					c = 0xEDB88320 ^ (c >> 1)
				} else {
					c = c >> 1
				}
			}
			crcTable[n] = c
		}
	}

	c := uint32(0) ^ 0xffffffff
	endOffset := len(stream)
	for i := 0; i < endOffset; i++ {
		c = crcTable[(c^uint32(stream[i]))&255] ^ (c >> 8)
	}

	return c ^ 0xffffffff
}
func InjectTextChunkIntoPNG(infile io.Reader, outfile io.Writer, key string, contents string) error {
	return InjectChunkIntoPNG(infile, outfile, "tEXt", []byte(key+"\x00"+contents))
}

// private chunks: with lowercase second letters
func InjectChunkIntoPNG(infile io.Reader, outfile io.Writer, chunkName string, contents []byte) error {
	sig := make([]byte, len(pngSig))
	if _, err := io.ReadFull(infile, sig); err != nil || !strings.HasPrefix(string(sig), string(pngSig)) {
		return fmt.Errorf("invalid PNG signature")
	}

	outfile.Write(pngSig)

	didTextChunk := false

	for {
		var length uint32
		name := make([]byte, 4)
		var crc uint32

		if err := binary.Read(infile, binary.BigEndian, &length); err != nil {
			break
		}
		if _, err := io.ReadFull(infile, name); err != nil {
			break
		}

		buffer := make([]byte, length)
		if _, err := io.ReadFull(infile, buffer); err != nil {
			break
		}

		if err := binary.Read(infile, binary.BigEndian, &crc); err != nil {
			break
		}
		if string(name) == "IEND" {
			if !didTextChunk {
				injectChunk(chunkName, contents, outfile)
				didTextChunk = true
			}
		}

		binary.Write(outfile, binary.BigEndian, length)
		outfile.Write(name)
		outfile.Write(buffer)
		binary.Write(outfile, binary.BigEndian, crc)
	}
	return nil
}

func ReadChunkFromPNG(infile io.ReadSeeker, nameOfChunk string) (output []byte) {
	ReadChunksFromPNG(infile, func(chunkName string, bufferPromise BytePromise) (continueReading bool) {
		if nameOfChunk == chunkName {
			output, _ = bufferPromise()
			return false
		}
		return true
	})
	return
}
func ReadTextChunkFromPNG(infile io.ReadSeeker, key string) (output string) {
	ReadChunksFromPNG(infile, func(chunkName string, buffer BytePromise) (continueReading bool) {
		if chunkName == "tEXt" {
			bytes, err := buffer()
			if err != nil {
				return false
			}
			parts := strings.SplitN(string(bytes), "\x00", 2)
			if parts[0] == key {
				output = parts[1]
				return false
			}
		}
		return true
	})
	return
}

type BytePromise func() ([]byte, error)
type ChunkVisitor func(chunkName string, bufferPromise BytePromise) (continueReading bool)

func ReadChunksFromPNG(infile io.ReadSeeker, visitor ChunkVisitor) error {
	sig := make([]byte, len(pngSig))
	if _, err := io.ReadFull(infile, sig); err != nil || !strings.HasPrefix(string(sig), string(pngSig)) {
		return fmt.Errorf("invalid PNG signature")
	}

	for {
		var length uint32
		name := make([]byte, 4)
		bufferWasRead := false
		var crc uint32

		if err := binary.Read(infile, binary.BigEndian, &length); err != nil {
			return nil
		}

		if _, err := io.ReadFull(infile, name); err != nil {
			return nil
		}

		bufferPromise := func() ([]byte, error) {
			buffer := make([]byte, length)
			_, err := io.ReadFull(infile, buffer)
			bufferWasRead = true
			return buffer, err
		}

		if !visitor(string(name), bufferPromise) {
			return nil
		}

		if !bufferWasRead {
			infile.Seek(int64(length), io.SeekCurrent)
		}

		if err := binary.Read(infile, binary.BigEndian, &crc); err != nil {
			return nil
		}
	}
}

// Copyright (c) 2020-2021 Shivaram Lingamneni
// released under the MIT license

package ircreader

import (
	"bytes"
	"errors"
	"io"
)

/*
IRCReader is an optimized line reader for IRC lines containing tags;
most IRC lines will not approach the maximum line length (8191 bytes
of tag data, plus 512 bytes of message data), so we want a buffered
reader that can start with a smaller buffer and expand if necessary,
while also maintaining a hard upper limit on the size of the buffer.
*/

var (
	ErrReadQ = errors.New("readQ exceeded (read too many bytes without terminating newline)")
)

type IRCReader struct {
	conn io.Reader

	initialSize int
	maxSize     int

	buf        []byte
	start      int // start of valid (i.e., read but not yet consumed) data in the buffer
	end        int // end of valid data in the buffer
	searchFrom int // start of valid data in the buffer not yet searched for \n
	eof        bool
}

// Returns a new *IRCReader with sane buffer size limits.
func NewIRCReader(conn io.Reader) *IRCReader {
	var reader IRCReader
	reader.Initialize(conn, 512, 8192+1024)
	return &reader
}

// "Placement new" for an IRCReader; initializes it with custom buffer size
// limits.
func (cc *IRCReader) Initialize(conn io.Reader, initialSize, maxSize int) {
	*cc = IRCReader{}
	cc.conn = conn
	cc.initialSize = initialSize
	cc.maxSize = maxSize
}

// Blocks until a full IRC line is read, then returns it. Accepts either \n
// or \r\n as the line terminator (but not \r in isolation). Passes through
// errors from the underlying connection. Returns ErrReadQ if the buffer limit
// was exceeded without a terminating \n.
func (cc *IRCReader) ReadLine() ([]byte, error) {
	for {
		// try to find a terminated line in the buffered data already read
		nlidx := bytes.IndexByte(cc.buf[cc.searchFrom:cc.end], '\n')
		if nlidx != -1 {
			// got a complete line
			line := cc.buf[cc.start : cc.searchFrom+nlidx]
			cc.start = cc.searchFrom + nlidx + 1
			cc.searchFrom = cc.start
			return line, nil
		}

		if cc.start == 0 && len(cc.buf) == cc.maxSize {
			return nil, ErrReadQ // out of space, can't expand or slide
		}

		if cc.eof {
			return nil, io.EOF
		}

		if len(cc.buf) < cc.maxSize && (len(cc.buf)-(cc.end-cc.start) < cc.initialSize/2) {
			// allocate a new buffer, copy any remaining data
			newLen := roundUpToPowerOfTwo(len(cc.buf) + 1)
			if newLen > cc.maxSize {
				newLen = cc.maxSize
			} else if newLen < cc.initialSize {
				newLen = cc.initialSize
			}
			newBuf := make([]byte, newLen)
			copy(newBuf, cc.buf[cc.start:cc.end])
			cc.buf = newBuf
		} else if cc.start != 0 {
			// slide remaining data back to the front of the buffer
			copy(cc.buf, cc.buf[cc.start:cc.end])
		}
		cc.end = cc.end - cc.start
		cc.start = 0

		cc.searchFrom = cc.end
		n, err := cc.conn.Read(cc.buf[cc.end:])
		cc.end += n
		if n != 0 && err == io.EOF {
			// we may have received new \n-terminated lines, try to parse them
			cc.eof = true
		} else if err != nil {
			return nil, err
		}
	}
}

// return n such that v <= n and n == 2**i for some i
func roundUpToPowerOfTwo(v int) int {
	// http://graphics.stanford.edu/~seander/bithacks.html
	v -= 1
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	return v + 1
}

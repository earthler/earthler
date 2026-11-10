package copies

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// A copy file starts with a random salt. From the copy key and the salt, a
// key for that file alone is derived, and the content follows in blocks,
// each sealed on its own with the number of the block and the name of the
// file, so a block cannot be moved, repeated or taken from another file,
// and the last block carries a mark, so a file cannot be cut short.
const (
	saltSize  = 32
	tagSize   = 16
	nonceSize = 12
	purpose   = "earthler copy v1"
)

// blockSize is the size of one block of content. A variable so tests can
// use small blocks.
var blockSize = 4 << 20

var errCut = errors.New("copies: the file is cut short")

func fileCipher(key Key, salt []byte) (cipher.AEAD, error) {
	k, err := hkdf.Key(sha256.New, key[:], salt, purpose, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func nonce(i uint64) []byte {
	n := make([]byte, nonceSize)
	binary.BigEndian.PutUint64(n[4:], i)
	return n
}

// extra is what each block is tied to besides its content: the name of the
// file in the copy and whether the block is the last one.
func extra(name string, last bool) []byte {
	b := append([]byte(name), 0)
	if last {
		return append(b, 1)
	}
	return append(b, 0)
}

// Encrypt writes src into dst as the copy file called name.
func Encrypt(dst io.Writer, name string, src io.Reader, key Key) error {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	aead, err := fileCipher(key, salt)
	if err != nil {
		return err
	}
	if _, err := dst.Write(salt); err != nil {
		return err
	}
	buf := make([]byte, blockSize)
	out := make([]byte, 0, blockSize+tagSize)
	for i := uint64(0); ; i++ {
		n, err := io.ReadFull(src, buf)
		last := false
		switch {
		case err == nil:
		case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
			last = true
		default:
			return err
		}
		out = aead.Seal(out[:0], nonce(i), buf[:n], extra(name, last))
		if _, err := dst.Write(out); err != nil {
			return err
		}
		if last {
			return nil
		}
	}
}

// Decrypt reads the copy file called name from src and writes its content
// into dst. It fails on a file that was changed, cut, or made with another
// key or under another name.
func Decrypt(dst io.Writer, name string, src io.Reader, key Key) error {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(src, salt); err != nil {
		return errCut
	}
	aead, err := fileCipher(key, salt)
	if err != nil {
		return err
	}
	br := bufio.NewReaderSize(src, 64<<10)
	chunk := make([]byte, blockSize+tagSize)
	plain := make([]byte, 0, blockSize)
	for i := uint64(0); ; i++ {
		n, err := io.ReadFull(br, chunk)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return err
		}
		if n < tagSize {
			return errCut
		}
		last := n < len(chunk)
		if !last {
			if _, err := br.Peek(1); errors.Is(err, io.EOF) {
				last = true // a full block at the very end: only valid if it was sealed as the last
			}
		}
		plain, err = aead.Open(plain[:0], nonce(i), chunk[:n], extra(name, last))
		if err != nil {
			return fmt.Errorf("copies: %s was changed, cut, or made with another key", name)
		}
		if _, err := dst.Write(plain); err != nil {
			return err
		}
		if last {
			return nil
		}
	}
}

// Package journal is the community's record of everything that happened: an
// append-only set of files where each entry is sealed to the one before it.
// Everything else in the system can be rebuilt from it.
//
// The format is described in docs/design/journal.md.
package journal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// MaxContent is the largest content of one entry, in bytes.
const MaxContent = 1 << 20

// erasedMark takes the place of a content that was erased. The digest stays
// in the line, so the chain of seals remains valid.
const erasedMark = "-"

// Seal is a SHA-256 value. It is used both for the digest of a content and
// for the seal that ties an entry to the previous one.
type Seal [sha256.Size]byte

func (s Seal) String() string { return hex.EncodeToString(s[:]) }

// Entry is one line of the journal.
type Entry struct {
	Number   uint64 // position in the journal, starting at 1
	PrevSeal Seal   // seal of the entry before this one; zero for entry 1
	Digest   Seal   // SHA-256 of the content
	Content  []byte // one JSON object; nil when the entry was erased
	Erased   bool
}

// Seal returns the seal of the entry: the SHA-256 of its number, the seal of
// the previous entry and the digest of its content, exactly as the three are
// written in the file. The content itself is not part of the seal, which is
// what lets a content be erased without breaking the chain.
func (e Entry) Seal() Seal {
	return sha256.Sum256(appendHeader(nil, e.Number, e.PrevSeal, e.Digest))
}

// appendHeader writes "number<TAB>previous seal<TAB>digest".
func appendHeader(buf []byte, number uint64, prev, digest Seal) []byte {
	buf = strconv.AppendUint(buf, number, 10)
	buf = append(buf, '\t')
	buf = hex.AppendEncode(buf, prev[:])
	buf = append(buf, '\t')
	buf = hex.AppendEncode(buf, digest[:])
	return buf
}

// appendLine writes a whole line, with the final newline.
func appendLine(buf []byte, e Entry) []byte {
	buf = appendHeader(buf, e.Number, e.PrevSeal, e.Digest)
	buf = append(buf, '\t')
	if e.Erased {
		buf = append(buf, erasedMark...)
	} else {
		buf = append(buf, e.Content...)
	}
	return append(buf, '\n')
}

// parseLine reads one line, without its final newline. It checks the shape
// of the line and that the content matches its digest. It does not check the
// number or the previous seal against the neighbouring lines.
func parseLine(line []byte) (Entry, error) {
	var e Entry
	fields := bytes.SplitN(line, []byte{'\t'}, 4)
	if len(fields) != 4 {
		return e, errors.New("the line does not have four fields")
	}
	n, err := strconv.ParseUint(string(fields[0]), 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != string(fields[0]) {
		return e, errors.New("the entry number is not valid")
	}
	e.Number = n
	if e.PrevSeal, err = parseSeal(fields[1]); err != nil {
		return e, fmt.Errorf("previous seal: %w", err)
	}
	if e.Digest, err = parseSeal(fields[2]); err != nil {
		return e, fmt.Errorf("digest: %w", err)
	}
	if string(fields[3]) == erasedMark {
		e.Erased = true
		return e, nil
	}
	if sha256.Sum256(fields[3]) != e.Digest {
		return e, errors.New("the content does not match its digest")
	}
	e.Content = bytes.Clone(fields[3])
	return e, nil
}

func parseSeal(text []byte) (Seal, error) {
	var s Seal
	if len(text) != hex.EncodedLen(len(s)) {
		return s, errors.New("wrong length")
	}
	if _, err := hex.Decode(s[:], text); err != nil {
		return s, err
	}
	if s.String() != string(text) {
		return s, errors.New("not in lowercase")
	}
	return s, nil
}

// CheckContent reports whether a content can be an entry: one JSON object,
// on one line, within the size limit. Append runs the same check.
func CheckContent(content []byte) error {
	switch {
	case len(content) == 0:
		return errors.New("journal: empty content")
	case len(content) > MaxContent:
		return fmt.Errorf("journal: content has %d bytes, the limit is %d", len(content), MaxContent)
	case content[0] != '{':
		return errors.New("journal: content must be a JSON object")
	case bytes.IndexByte(content, '\n') >= 0:
		return errors.New("journal: content must fit in one line")
	case !json.Valid(content):
		return errors.New("journal: content is not valid JSON")
	}
	return nil
}

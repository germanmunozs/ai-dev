package gitmanager

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
)

// indexEntry is one stage-0 entry of a Git index.
type indexEntry struct {
	path string
	mode uint32
	oid  ObjectOID
}

// errIndex reports an index that is not exactly an autonomous version-2
// index (F008-FR-157–158).
var errIndex = fmt.Errorf("%w: index is not an autonomous version-2 index", errInvalidModel)

// parseAutonomousIndex parses a Git index independently of Git and accepts
// only what the prepared index may be: the DIRC signature, version 2, a
// valid trailing checksum, sorted unique stage-0 entries without extended
// flags, and no extension other than the cache tree. A link (split index)
// or sharedindex dependency, a sparse directory entry, an untracked cache
// or fsmonitor extension, or any trailing byte is refused.
func parseAutonomousIndex(data []byte, format ObjectFormat) ([]indexEntry, error) {
	var h hash.Hash = sha1.New()
	if format == ObjectFormatSHA256 {
		h = sha256.New()
	}
	size := h.Size()
	if len(data) < 12+size || string(data[:4]) != "DIRC" || binary.BigEndian.Uint32(data[4:8]) != 2 {
		return nil, errIndex
	}
	end := len(data) - size
	h.Write(data[:end])
	if !bytes.Equal(h.Sum(nil), data[end:]) {
		return nil, fmt.Errorf("%w: checksum", errIndex)
	}
	count := binary.BigEndian.Uint32(data[8:12])
	entries := make([]indexEntry, 0, min(int(count), end/62))
	offset := 12
	for i := uint32(0); i < count; i++ {
		fixed := 40 + size + 2
		if offset+fixed > end {
			return nil, fmt.Errorf("%w: truncated entry", errIndex)
		}
		mode := binary.BigEndian.Uint32(data[offset+24:])
		flags := binary.BigEndian.Uint16(data[offset+40+size:])
		if flags&0xf000 != 0 {
			// Extended (v3), assume-valid or stage bits.
			return nil, fmt.Errorf("%w: entry flags", errIndex)
		}
		nul := bytes.IndexByte(data[offset+fixed:end], 0)
		if nul <= 0 || (flags&0x0fff != 0x0fff && int(flags&0x0fff) != nul) {
			return nil, fmt.Errorf("%w: entry name", errIndex)
		}
		entry := indexEntry{
			path: string(data[offset+fixed : offset+fixed+nul]), mode: mode,
			oid: ObjectOID(hex.EncodeToString(data[offset+40 : offset+40+size])),
		}
		if mode != 0o100644 && mode != 0o100755 {
			return nil, fmt.Errorf("%w: entry mode", errIndex)
		}
		if len(entries) > 0 && entries[len(entries)-1].path >= entry.path {
			return nil, fmt.Errorf("%w: entry order", errIndex)
		}
		entries = append(entries, entry)
		offset += (fixed + nul + 8) &^ 7
		if offset > end {
			return nil, fmt.Errorf("%w: truncated entry", errIndex)
		}
	}
	for offset < end {
		if offset+8 > end {
			return nil, fmt.Errorf("%w: truncated extension", errIndex)
		}
		signature, length := string(data[offset:offset+4]), int(binary.BigEndian.Uint32(data[offset+4:offset+8]))
		if signature != "TREE" || length > end-offset-8 {
			return nil, fmt.Errorf("%w: extension %q", errIndex, signature)
		}
		offset += 8 + length
	}
	return entries, nil
}

// indexHoldsTree reports whether data is an autonomous index whose
// entries are exactly the tree: the tree is re-encoded in process from the
// entries, so equal OIDs prove equal paths, modes and blobs.
func indexHoldsTree(data []byte, format ObjectFormat, tree ObjectOID) bool {
	entries, err := parseAutonomousIndex(data, format)
	if err != nil || len(entries) == 0 {
		return false
	}
	files := make([]AuthorizedFile, len(entries))
	for i, entry := range entries {
		files[i] = AuthorizedFile{PathUTF8: entry.path, Mode: strconv.FormatUint(uint64(entry.mode), 8), BlobOID: BlobOID(entry.oid)}
	}
	trees, err := commitTrees(format, files)
	return err == nil && trees[len(trees)-1].oid == tree
}

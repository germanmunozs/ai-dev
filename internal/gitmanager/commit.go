package gitmanager

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"
	"strconv"
	"strings"
	"time"
)

// gitObject is one Git object of the initial commit, encoded in process so
// the plan can freeze its exact OID before any object exists.
type gitObject struct {
	kind    string // blob, tree or commit
	content []byte
	oid     ObjectOID
}

// objectOID hashes an object exactly as Git does: "<kind> <size>\0" then
// the content, under the repository's object format.
func objectOID(format ObjectFormat, kind string, content []byte) ObjectOID {
	var h hash.Hash = sha1.New()
	if format == ObjectFormatSHA256 {
		h = sha256.New()
	}
	fmt.Fprintf(h, "%s %d\x00", kind, len(content))
	h.Write(content)
	return ObjectOID(hex.EncodeToString(h.Sum(nil)))
}

// blobOID is the OID Git gives exact bytes hashed without filters.
func blobOID(format ObjectFormat, data []byte) BlobOID {
	return BlobOID(objectOID(format, "blob", data))
}

// commitTrees encodes the exact tree hierarchy of the authorized files
// (F008-FR-111): one tree per directory, entries in Git order, every
// subtree before its parent. The root tree is last.
func commitTrees(format ObjectFormat, files []AuthorizedFile) ([]gitObject, error) {
	type entry struct {
		name, mode string
		oid        ObjectOID
	}
	directories := map[string][]entry{"": nil}
	for _, file := range files {
		parts := strings.Split(file.PathUTF8, "/")
		for i := 1; i < len(parts); i++ {
			if _, ok := directories[strings.Join(parts[:i], "/")]; !ok {
				directories[strings.Join(parts[:i], "/")] = nil
			}
		}
		parent := strings.Join(parts[:len(parts)-1], "/")
		directories[parent] = append(directories[parent], entry{name: parts[len(parts)-1], mode: file.Mode, oid: ObjectOID(file.BlobOID)})
	}
	// Deeper directories first, so each subtree OID exists before its
	// parent is encoded.
	names := make([]string, 0, len(directories))
	for name := range directories {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		di, dj := depth(names[i]), depth(names[j])
		if di != dj {
			return di > dj
		}
		return names[i] < names[j]
	})
	var trees []gitObject
	for _, name := range names {
		entries := directories[name]
		// Git orders entries by name, a subtree compared as name + "/".
		key := func(e entry) string {
			if e.mode == "40000" {
				return e.name + "/"
			}
			return e.name
		}
		sort.Slice(entries, func(i, j int) bool { return key(entries[i]) < key(entries[j]) })
		var content []byte
		seen := map[string]bool{}
		for _, e := range entries {
			if seen[e.name] {
				return nil, fmt.Errorf("%w: a path is both a file and a directory", errInvalidModel)
			}
			seen[e.name] = true
			raw, err := hex.DecodeString(string(e.oid))
			if err != nil {
				return nil, fmt.Errorf("%w: tree entry OID", errInvalidModel)
			}
			content = append(content, e.mode+" "+e.name+"\x00"...)
			content = append(content, raw...)
		}
		tree := gitObject{kind: "tree", content: content, oid: objectOID(format, "tree", content)}
		trees = append(trees, tree)
		if name != "" {
			parent, base := "", name
			if at := strings.LastIndexByte(name, '/'); at >= 0 {
				parent, base = name[:at], name[at+1:]
			}
			directories[parent] = append(directories[parent], entry{name: base, mode: "40000", oid: tree.oid})
		}
	}
	return trees, nil
}

func depth(dir string) int {
	if dir == "" {
		return 0
	}
	return strings.Count(dir, "/") + 1
}

// commitObject encodes the unsigned root commit exactly as commit-tree
// writes it from the frozen identities, dates and message: no parent, no
// encoding header and no signature (F008-FR-096–108).
func commitObject(format ObjectFormat, tree ObjectOID, intent ImmutableIntent) gitObject {
	content := "tree " + string(tree) + "\n" +
		"author " + identityLine(intent.Author, intent.AuthorTimestamp) + "\n" +
		"committer " + identityLine(intent.Committer, intent.CommitterTimestamp) + "\n\n" +
		string(intent.CommitMessageBytes())
	return gitObject{kind: "commit", content: []byte(content), oid: objectOID(format, "commit", []byte(content))}
}

func identityLine(identity GitIdentity, at time.Time) string {
	return identity.Name + " <" + identity.Email + "> " + strconv.FormatInt(at.Unix(), 10) + " " + at.Format("-0700")
}

// commitPlan is the complete object set of an initial commit.
type commitPlan struct {
	blobs  []AuthorizedFile
	trees  []gitObject
	commit gitObject
}

func planCommit(intent ImmutableIntent) (commitPlan, error) {
	files := intent.AuthorizedFiles()
	trees, err := commitTrees(intent.ObjectFormat, files)
	if err != nil {
		return commitPlan{}, err
	}
	root := trees[len(trees)-1].oid
	return commitPlan{blobs: files, trees: trees, commit: commitObject(intent.ObjectFormat, root, intent)}, nil
}

// rootTree is the OID of the commit's root tree.
func (p commitPlan) rootTree() ObjectOID { return p.trees[len(p.trees)-1].oid }

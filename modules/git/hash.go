// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"regexp"
	"strconv"
	"strings"
)

type HashID int

const (
	Sha1 HashID = iota
	Sha256
)

// sha1Pattern can be used to determine if a string is an valid sha
var sha1Pattern = regexp.MustCompile(`^[0-9a-f]{4,40}$`)

// sha256Pattern can be used to determine if a string is an valid sha
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{4,64}$`)

type Hash interface {
	String() string
	IsZero() bool
	RawValue() []byte
	Type() HashType
}

type HashType interface {
	ID() HashID
	String() string

	// Empty is the hash of empty git
	Empty() Hash
	// EmptyTree is the hash of an empty tree
	EmptyTree() Hash
	// FullLength is the length of the hash's hex string
	FullLength() int

	IsValid(input string) bool
	MustID(b []byte) Hash
	MustIDFromString(s string) Hash
	NewID(b []byte) (Hash, error)
	NewIDFromString(s string) (Hash, error)
	NewEmptyID() Hash

	NewHasher() HasherInterface
}

/* SHA1 */
type Sha1Hash [20]byte

func (h *Sha1Hash) String() string {
	return hex.EncodeToString(h[:])
}

func (h *Sha1Hash) IsZero() bool {
	empty := Sha1Hash{}
	return bytes.Equal(empty[:], h[:])
}
func (h *Sha1Hash) RawValue() []byte { return h[:] }
func (*Sha1Hash) Type() HashType     { return &Sha1HashType{} }

/* SHA1 Type */
type Sha1HashType struct{}

func (*Sha1HashType) ID() HashID     { return Sha1 }
func (*Sha1HashType) String() string { return "sha1" }
func (*Sha1HashType) Empty() Hash    { return &Sha1Hash{} }
func (*Sha1HashType) EmptyTree() Hash {
	return &Sha1Hash{
		0x4b, 0x82, 0x5d, 0xc6, 0x42, 0xcb, 0x6e, 0xb9, 0xa0, 0x60,
		0xe5, 0x4b, 0xf8, 0xd6, 0x92, 0x88, 0xfb, 0xee, 0x49, 0x04,
	}
}
func (*Sha1HashType) FullLength() int { return 40 }
func (*Sha1HashType) IsValid(input string) bool {
	return sha1Pattern.MatchString(input)
}

func (*Sha1HashType) MustID(b []byte) Hash {
	var id Sha1Hash
	copy(id[0:20], b)
	return &id
}

func (h *Sha1HashType) MustIDFromString(s string) Hash {
	return hashMustFromString(h, s)
}

func (h *Sha1HashType) NewID(b []byte) (Hash, error) {
	return hashFromRaw(h, b)
}

func (h *Sha1HashType) NewIDFromString(s string) (Hash, error) {
	return hashFromString(h, s)
}

func (*Sha1HashType) NewEmptyID() Hash {
	return NewSha1()
}

func (h *Sha1HashType) NewHasher() HasherInterface {
	return &Sha1Hasher{sha1.New()}
}

func NewSha1() *Sha1Hash {
	return &Sha1Hash{}
}

/* SHA256 */
type Sha256Hash [32]byte

func (h *Sha256Hash) String() string { return hex.EncodeToString(h[:]) }
func (h *Sha256Hash) IsZero() bool {
	empty := Sha256Hash{}
	return bytes.Equal(empty[:], h[:])
}
func (h *Sha256Hash) RawValue() []byte { return h[:] }
func (*Sha256Hash) Type() HashType     { return &Sha256HashType{} }

/* SHA256 Type */

type Sha256HashType struct{}

func (*Sha256HashType) ID() HashID     { return Sha256 }
func (*Sha256HashType) String() string { return "sha256" }
func (*Sha256HashType) Empty() Hash    { return &Sha256Hash{} }
func (*Sha256HashType) EmptyTree() Hash {
	return &Sha256Hash{
		0x6e, 0xf1, 0x9b, 0x41, 0x22, 0x5c, 0x53, 0x69, 0xf1, 0xc1,
		0x04, 0xd4, 0x5d, 0x8d, 0x85, 0xef, 0xa9, 0xb0, 0x57, 0xb5,
		0x3b, 0x14, 0xb4, 0xb9, 0xb9, 0x39, 0xdd, 0x74, 0xde, 0xcc,
		0x53, 0x21,
	}
}
func (*Sha256HashType) FullLength() int { return 64 }
func (*Sha256HashType) IsValid(input string) bool {
	return sha256Pattern.MatchString(input)
}

func (*Sha256HashType) MustID(b []byte) Hash {
	var id Sha256Hash
	copy(id[0:32], b)
	return &id
}

func (h *Sha256HashType) MustIDFromString(s string) Hash {
	return hashMustFromString(h, s)
}

func (h *Sha256HashType) NewID(b []byte) (Hash, error) {
	return hashFromRaw(h, b)
}

func (h *Sha256HashType) NewIDFromString(s string) (Hash, error) {
	return hashFromString(h, s)
}

func (*Sha256HashType) NewEmptyID() Hash {
	return NewSha256()
}

func (h *Sha256HashType) NewHasher() HasherInterface {
	return &Sha256Hasher{sha256.New()}
}

func NewSha256() *Sha256Hash {
	return &Sha256Hash{}
}

// generic implementations
func NewHash(hash string) (Hash, error) {
	hash = strings.ToLower(hash)
	switch hash {
	case "sha1":
		return &Sha1Hash{}, nil
	case "sha256":
		return &Sha256Hash{}, nil
	}

	return nil, errors.New("unsupported hash type")
}

func hashFromRaw(h HashType, b []byte) (Hash, error) {
	if len(b) != h.FullLength()/2 {
		return h.Empty(), fmt.Errorf("length must be %d: %v", h.FullLength(), b)
	}
	return h.MustID(b), nil
}

func hashMustFromString(h HashType, s string) Hash {
	b, _ := hex.DecodeString(s)
	return h.MustID(b)
}

func hashFromString(h HashType, s string) (Hash, error) {
	s = strings.TrimSpace(s)
	if len(s) != h.FullLength() {
		return h.Empty(), fmt.Errorf("length must be %d: %s", h.FullLength(), s)
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return h.Empty(), err
	}
	return h.NewID(b)
}

// utils
func HashTypeFromID(id HashID) HashType {
	switch id {
	case Sha1:
		return &Sha1HashType{}
	case Sha256:
		return &Sha256HashType{}
	}

	return nil
}

func HashTypeFromString(hash string) (HashType, error) {
	switch strings.ToLower(hash) {
	case "sha1":
		return &Sha1HashType{}, nil
	case "sha256":
		return &Sha256HashType{}, nil
	}

	return nil, fmt.Errorf("unknown hash type: %s", hash)
}

func HashFromString(hexHash string) (Hash, error) {
	switch len(hexHash) {
	case 40:
		hashType := Sha1HashType{}
		h, err := hashType.NewIDFromString(hexHash)
		if err != nil {
			return nil, err
		}
		return h, nil
	case 64:
		hashType := Sha256HashType{}
		h, err := hashType.NewIDFromString(hexHash)
		if err != nil {
			return nil, err
		}
		return h, nil
	}

	return nil, fmt.Errorf("invalid hash hex string: '%s' len: %d", hexHash, len(hexHash))
}

// HashInterface is a struct that will generate a Hash
type HasherInterface interface {
	hash.Hash

	HashSum() Hash
}

type Sha1Hasher struct {
	hash.Hash
}
type Sha256Hasher struct {
	hash.Hash
}

// ComputeBlobHash compute the hash for a given blob content
func ComputeBlobHash(hashType HashType, content []byte) Hash {
	return ComputeHash(hashType, ObjectBlob, content)
}

// ComputeHash compute the hash for a given ObjectType and content
func ComputeHash(hashType HashType, t ObjectType, content []byte) Hash {
	h := hashType.NewHasher()
	_, _ = h.Write(t.Bytes())
	_, _ = h.Write([]byte(" "))
	_, _ = h.Write([]byte(strconv.FormatInt(int64(len(content)), 10)))
	_, _ = h.Write([]byte{0})
	return h.HashSum()
}

// Sum generates a SHA1 for the provided hash
func (h *Sha1Hasher) HashSum() Hash {
	var sha1 Sha1Hash
	copy(sha1[:], h.Hash.Sum(nil))
	return &sha1
}

// Sum generates a SHA256 for the provided hash
func (h *Sha256Hasher) HashSum() Hash {
	var sha256 Sha256Hash
	copy(sha256[:], h.Hash.Sum(nil))
	return &sha256
}

type ErrInvalidSHA struct {
	SHA string
}

func (err ErrInvalidSHA) Error() string {
	return fmt.Sprintf("invalid sha: %s", err.SHA)
}

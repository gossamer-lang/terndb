// Package bench holds what the bbolt programs share: the key encoding, the
// stored row, and the id sequence every benchmark target walks.
package bench

import (
	"encoding/binary"
	"strconv"
)

// Key is the id as eight big-endian bytes, the encoding the other key-value
// targets use.
func Key(id uint64) []byte {
	var k [8]byte
	binary.BigEndian.PutUint64(k[:], id)
	return k[:]
}

// Row is the row as its finished JSON, the shape rust-redb, cpp-rocksdb, c-lmdb,
// and go-pogreb store.
func Row(id uint64) []byte {
	n := strconv.FormatUint(id, 10)
	return []byte(`{"id":` + n + `,"name":"user-` + n + `","email":"user-` + n +
		`@example.com","score":1.5,"active":true}`)
}

// Stride and Batch fix the id sequence every target walks, so each reads the
// same rows in the same order.
const (
	Stride = 2654435761
	Batch  = 1024
)

// IDAt is the i-th id of the shared sequence over rows ids.
func IDAt(i, rows int64) uint64 { return uint64((i*Stride)%rows + 1) }

// Users is the bucket the rows live in.
var Users = []byte("users")

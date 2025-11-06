package internal

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	bolt "go.etcd.io/bbolt"
)

// BoltDB-backed simple compatibility layer for the small subset of list/hash operations
// used in Trailarr. This is intentionally minimal and synchronous.

type BoltClient struct {
	db *bolt.DB
}

var boltClient *BoltClient
var boltOnce sync.Once

func openBoltDB() (*BoltClient, error) {
	// Prevent test runs from creating the real on-disk DB when the default
	// TrailarrRoot is being used. Tests should set TrailarrRoot to a temp
	// directory (see many tests in internal/*_test.go). If we detect we're in
	// a test binary and TrailarrRoot is the default system path, return an
	// error so callers fall back to the in-memory implementation.
	if isTestBinary() && TrailarrRoot == "/var/lib/trailarr" {
		return nil, fmt.Errorf("refusing to open on-disk bolt DB during tests when TrailarrRoot=%s", TrailarrRoot)
	}

	dbPath := filepath.Join(TrailarrRoot, "trailarr.db")
	// Ensure parent directory exists to avoid surprising errors when using a
	// non-default TrailarrRoot. (When using the default and not running
	// tests, the directory is expected to exist or the service has
	// permissions to create it.)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, err
	}
	return &BoltClient{db: db}, nil
}

// isTestBinary attempts to detect if the current process is a 'go test'
// binary. This is a heuristic (checks the executable name) used to avoid
// opening the real on-disk DB during unit tests when the default root is in
// a system path.
func isTestBinary() bool {
	// os.Args[0] for 'go test' executed binaries usually ends with '.test'
	if strings.HasSuffix(os.Args[0], ".test") {
		return true
	}
	// Also honor GOTEST environment variable if set by some CI or wrappers
	if v := os.Getenv("GOTEST"); v != "" {
		return true
	}
	return false
}

// GetBoltClient returns a singleton BoltClient
func GetBoltClient() (*BoltClient, error) {
	var err error
	boltOnce.Do(func() {
		boltClient, err = openBoltDB()
	})
	return boltClient, err
}

var ErrNotFound = errors.New("not found")

// Ping is a no-op for BoltDB
func (c *BoltClient) Ping(ctx context.Context) error {
	return nil
}

// ------------ string key/value (simple KV) ----------------
func (c *BoltClient) Set(ctx context.Context, key string, value []byte) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("kv"))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), value)
	})
}

func (c *BoltClient) Get(ctx context.Context, key string) (string, error) {
	var out []byte
	err := c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("kv"))
		if b == nil {
			return ErrNotFound
		}
		v := b.Get([]byte(key))
		if v == nil {
			return ErrNotFound
		}
		out = append([]byte(nil), v...)
		return nil
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ------------ hash (HSET/HGET/HVALS/HDEL) ----------------
func hashBucketName(key string) []byte { return []byte("hash:" + key) }

func (c *BoltClient) HSet(ctx context.Context, key, field string, value []byte) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(hashBucketName(key))
		if err != nil {
			return err
		}
		return b.Put([]byte(field), value)
	})
}

func (c *BoltClient) HGet(ctx context.Context, key, field string) (string, error) {
	var out []byte
	err := c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(hashBucketName(key))
		if b == nil {
			return ErrNotFound
		}
		v := b.Get([]byte(field))
		if v == nil {
			return ErrNotFound
		}
		out = append([]byte(nil), v...)
		return nil
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (c *BoltClient) HVals(ctx context.Context, key string) ([]string, error) {
	var vals []string
	err := c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(hashBucketName(key))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			vals = append(vals, string(v))
			return nil
		})
	})
	return vals, err
}

func (c *BoltClient) HDel(ctx context.Context, key, field string) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(hashBucketName(key))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(field))
	})
}

// ------------ list (LRANGE, RPUSH, LTRIM, LSET, LREM, DEL) ----------------
func listBucketName(key string) []byte { return []byte("list:" + key) }

func u64ToBytes(i uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], i)
	return b[:]
}

func (c *BoltClient) RPush(ctx context.Context, key string, value []byte) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(listBucketName(key))
		if err != nil {
			return err
		}
		seq, _ := b.NextSequence()
		if err := b.Put(u64ToBytes(seq), value); err != nil {
			return err
		}
		// Diagnostic: count current entries in this list bucket
		var count int
		_ = b.ForEach(func(k, v []byte) error {
			count++
			return nil
		})
		TrailarrLog(DEBUG, "Bolt", "RPush key=%s seq=%d new_count=%d", key, seq, count)
		return nil
	})
}

func (c *BoltClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	var out []string
	err := c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(listBucketName(key))
		if b == nil {
			return nil
		}
		// Collect all items in order
		return b.ForEach(func(k, v []byte) error {
			out = append(out, string(v))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	// Apply start/stop semantics (support -1)
	n := int64(len(out))
	if n == 0 {
		return []string{}, nil
	}
	if start < 0 {
		start = n + start
	}
	if stop < 0 {
		stop = n + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop || start >= n {
		return []string{}, nil
	}
	return out[start : stop+1], nil
}

// listBucketKVs returns the key/value pairs in the bucket in iteration order.
// Keys and values are copied so callers may modify returned slices safely.
func listBucketKVs(b *bolt.Bucket) [][2][]byte {
	var kvs [][2][]byte
	if b == nil {
		return kvs
	}
	_ = b.ForEach(func(k, v []byte) error {
		kk := append([]byte(nil), k...)
		vv := append([]byte(nil), v...)
		kvs = append(kvs, [2][]byte{kk, vv})
		return nil
	})
	return kvs
}

func normalizeRange(n, start, stop int64) (int64, int64, bool) {
	if n == 0 {
		return 0, 0, true
	}
	if start < 0 {
		start = n + start
	}
	if stop < 0 {
		stop = n + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop {
		return 0, 0, true
	}
	return start, stop, false
}

func (c *BoltClient) LTrim(ctx context.Context, key string, start, stop int64) error {
	// Simplified implementation using helpers to reduce cognitive complexity.
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(listBucketName(key))
		if b == nil {
			return nil
		}
		kvs := listBucketKVs(b)
		s, e, empty := normalizeRange(int64(len(kvs)), start, stop)
		if empty {
			// clear bucket
			_ = tx.DeleteBucket(listBucketName(key))
			TrailarrLog(DEBUG, "Bolt", "LTrim key=%s resulted in empty bucket (start=%d stop=%d) -> deleted", key, start, stop)
			return nil
		}
		keep := kvs[s : e+1]
		// Delete keys outside the keep range to avoid recreating the bucket.
		// This preserves the bucket sequence counter so NextSequence() will
		// continue to produce increasing, non-colliding values.
		all := listBucketKVs(b)
		// delete keys before 's' and after 'e'
		for idx := int64(0); idx < int64(len(all)); idx++ {
			if idx < s || idx > e {
				k := all[idx][0]
				if err := b.Delete(k); err != nil {
					return err
				}
			}
		}
		TrailarrLog(DEBUG, "Bolt", "LTrim key=%s start=%d stop=%d kept=%d", key, s, e, len(keep))
		return nil
	})
}

func (c *BoltClient) LSet(ctx context.Context, key string, index int64, value []byte) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(listBucketName(key))
		if b == nil {
			return fmt.Errorf("index out of range")
		}
		// rebuild all into slice, set index, rewrite
		kvs := listBucketKVs(b)
		if index < 0 || index >= int64(len(kvs)) {
			return fmt.Errorf("index out of range")
		}
		// replace value at the same key
		kvs[index][1] = append([]byte(nil), value...)
		// Update the value at the key corresponding to the requested index
		// without recreating the bucket so the sequence counter remains stable.
		targetKey := kvs[index][0]
		if err := b.Put(targetKey, kvs[index][1]); err != nil {
			return err
		}
		TrailarrLog(DEBUG, "Bolt", "LSet key=%s index=%d total=%d", key, index, len(kvs))
		return nil
	})
}

// helper to remove up to count occurrences of value from vals (preserves original behavior for count <= 0)
func removeMatches(vals [][]byte, count int, value []byte) [][]byte {
	if len(vals) == 0 || count <= 0 {
		return vals
	}
	target := string(value)
	newVals := make([][]byte, 0, len(vals))
	removed := 0
	for _, v := range vals {
		if removed < count && string(v) == target {
			removed++
			continue
		}
		newVals = append(newVals, v)
	}
	return newVals
}

func (c *BoltClient) LRem(ctx context.Context, key string, count int, value []byte) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(listBucketName(key))
		if b == nil {
			return nil
		}
		var vals [][]byte
		_ = b.ForEach(func(k, v []byte) error {
			vals = append(vals, append([]byte(nil), v...))
			return nil
		})
		newVals := removeMatches(vals, count, value)
		_ = tx.DeleteBucket(listBucketName(key))
		if len(newVals) == 0 {
			return nil
		}
		nb, err := tx.CreateBucketIfNotExists(listBucketName(key))
		if err != nil {
			return err
		}
		for _, v := range newVals {
			// allocate keys via NextSequence so subsequent RPush calls don't collide
			seq, _ := nb.NextSequence()
			if err := nb.Put(u64ToBytes(seq), v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *BoltClient) Del(ctx context.Context, key string) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		_ = tx.DeleteBucket(listBucketName(key))
		_ = tx.DeleteBucket(hashBucketName(key))
		b := tx.Bucket([]byte("kv"))
		if b != nil {
			_ = b.Delete([]byte(key))
		}
		return nil
	})
}

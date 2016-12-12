package main

import (
	"math/rand"
	"time"

	"github.com/lib/pq"
)

// Embed the pq-provided NullTime in our own version, so we can override the
// MarshalJSON method to return clean

type nullTime struct {
	pq.NullTime
}

func toNullTime(t time.Time) nullTime {
	return nullTime{
		pq.NullTime{
			Time:  t,
			Valid: true,
		},
	}
}

func (nt *nullTime) MarshalJSON() ([]byte, error) {
	if !nt.Valid {
		return []byte("null"), nil
	}
	return nt.Time.MarshalJSON()
}

func randomString(length int) string {
	rand.Seed(time.Now().UTC().UnixNano())
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

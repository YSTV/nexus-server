package main

import (
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

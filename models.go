package main

import "github.com/lib/pq"

type stream struct {
	ID       int         `db:"id"`
	Name     string      `db:"name"`
	IsPublic bool        `db:"is_public"`
	StartAt  pq.NullTime `db:"start_at"`
	EndAt    pq.NullTime `db:"start_at"`
}

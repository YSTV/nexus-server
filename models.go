package main

type stream struct {
	ID       int      `db:"id" json:"id"`
	Name     string   `db:"name" json:"name"`
	IsPublic bool     `db:"is_public" json:"is_public"`
	StartAt  nullTime `db:"start_at" json:"start_at"`
	EndAt    nullTime `db:"end_at" json:"end_at"`
}

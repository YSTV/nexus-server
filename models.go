package main

// stream is a single video stream
type stream struct {
	ID          int      `db:"id" json:"id"`
	DisplayName string   `db:"display_name" json:"display_name"`
	IsPublic    bool     `db:"is_public" json:"is_public"`
	StartAt     nullTime `db:"start_at" json:"start_at"`
	EndAt       nullTime `db:"end_at" json:"end_at"`
	StreamName  string   `db:"stream_name" json:"stream_name"`
	Key         string   `db:"key" json:"key"`
}

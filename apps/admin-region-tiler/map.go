package main

// TileMap describes one configured tile source and its fetch policy.
type TileMap struct {
	ID          int
	Name        string
	SourceName  string
	Description string
	Schema      string // xyz or tms
	Min         int
	Max         int
	Format      string
	JSON        string
	URL         string
	Token       string
	Policy      FetchPolicy
}

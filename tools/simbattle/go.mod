module github.com/geone39/fireline/tools/simbattle

go 1.26.0

require (
	github.com/OWNER/fireline/sim v0.0.0
	github.com/geone39/fireline/server v0.0.0
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/go-chi/chi/v5 v5.5.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/OWNER/fireline/sim => ../../sim

replace github.com/geone39/fireline/server => ../../server

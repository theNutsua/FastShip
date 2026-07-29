package planner

// preset describes sensible defaults for a known service. Presets are a
// convenience — they let a user write "- postgres" instead of spelling out
// the image, port, data path, and env. They are NOT an allowlist: any
// image can be a service without a preset (the user just supplies the
// details themselves).
//
// Adding a known service is adding a row here. No code changes, no
// switch statements.
type preset struct {
	Image    string            // default image
	Port     int               // default listen port
	DataPath string            // where persistent data lives (empty = stateless)
	Env      map[string]string // default env; {{VAR}} placeholders get filled
	// ConnEnvVar is the env var the APP receives to reach this service,
	// e.g. DATABASE_URL. ConnFormat is the connection string template.
	ConnEnvVar string
	ConnFormat string // placeholders: {{user}} {{pass}} {{host}} {{db}}
}

// presets maps a known service name to its defaults. This is the entire
// "built-in services" list — extend it by adding rows.
var presets = map[string]preset{
	"postgres": {
		Image:    "docker.io/library/postgres:16-alpine",
		Port:     5432,
		DataPath: "/var/lib/postgresql/data",
		Env: map[string]string{
			"POSTGRES_USER":     "{{user}}",
			"POSTGRES_PASSWORD": "{{pass}}",
			"POSTGRES_DB":       "{{db}}",
		},
		ConnEnvVar: "DATABASE_URL",
		ConnFormat: "postgres://{{user}}:{{pass}}@{{host}}:5432/{{db}}?sslmode=disable",
	},
	"mysql": {
		Image:    "docker.io/library/mysql:8",
		Port:     3306,
		DataPath: "/var/lib/mysql",
		Env: map[string]string{
			"MYSQL_USER":          "{{user}}",
			"MYSQL_PASSWORD":      "{{pass}}",
			"MYSQL_DATABASE":      "{{db}}",
			"MYSQL_ROOT_PASSWORD": "{{pass}}",
		},
		ConnEnvVar: "DATABASE_URL",
		ConnFormat: "mysql://{{user}}:{{pass}}@{{host}}:3306/{{db}}",
	},
	"redis": {
		Image:      "docker.io/library/redis:7-alpine",
		Port:       6379,
		DataPath:   "/data",
		ConnEnvVar: "REDIS_URL",
		ConnFormat: "redis://{{host}}:6379",
	},
	"mongo": {
		Image:      "docker.io/library/mongo:7",
		Port:       27017,
		DataPath:   "/data/db",
		ConnEnvVar: "MONGO_URL",
		ConnFormat: "mongodb://{{host}}:27017/{{db}}",
	},
}

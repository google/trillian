//go:build postgresql || !(cloudspanner || crdb || mysql)

package provider

import (
	_ "github.com/google/trillian/storage/postgresql"

	_ "github.com/google/trillian/quota/postgresqlqm"
)

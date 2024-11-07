//go:build mysql || !(cloudspanner || crdb || postgresql)

package provider

import (
	_ "github.com/google/trillian/storage/mysql"

	_ "github.com/google/trillian/quota/mysqlqm"
)

//go:build crdb || !(cloudspanner || mysql || postgresql)

package provider

import (
	_ "github.com/google/trillian/storage/crdb"

	_ "github.com/google/trillian/quota/crdbqm"
)

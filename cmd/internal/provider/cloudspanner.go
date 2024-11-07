//go:build cloudspanner || !(crdb || mysql || postgresql)

package provider

import (
	_ "github.com/google/trillian/storage/cloudspanner"
)

package main

import (
	"context"
	"fmt"
	"time"
	
	"github.com/google/trillian/storage/postgres/testdb"

)
var (
	fContext context.Context
)

func main() {
	fmt.Println("Hello");
	fContext, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*30))
	defer cancel()
	testdb.PGAvailable();
	testdb.NewTrillianDB(fContext);
}


package main

import (
	"context"
	"github.com/iodasolutions/xbee-common/indus"
	"log"
)

func main() {
	ctx := context.TODO()

	if err := indus.Build(ctx, "main", "virtualbox"); err != nil {
		log.Fatal(err)
	}
}

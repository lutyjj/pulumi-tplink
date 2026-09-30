// Command pulumi-resource-tplink serves the TP-Link provider protocol.
package main

import (
	"context"
	"fmt"
	"github.com/lutyjj/pulumi-tplink/provider"
	"os"
)

func main() {
	if err := provider.Provider().Run(context.Background(), provider.Name, provider.Version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
